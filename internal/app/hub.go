package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/nats.go"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/hub/web"
	"github.com/zsomething/zlaw/internal/logging"
	"github.com/zsomething/zlaw/internal/messaging"
)

// runHub loads the hub config and starts the hub process. It blocks until ctx is cancelled.
func runHub(ctx context.Context, configPath string, externalNATSURL string, logger *slog.Logger, noColor bool, dashboardAddr string) error {
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return fmt.Errorf("load hub config: %w", err)
	}

	// Wrap logger with hub prefix and color.
	logger = setupHubLogger(logger, noColor)

	result, err := hub.StartNATS(ctx, cfg, externalNATSURL, logger)
	if err != nil {
		return fmt.Errorf("start nats: %w", err)
	}
	defer result.Conn.Close()

	// Create the durable agent inbox stream and pre-create consumers for
	// all configured agents.
	sm := hub.NewStreamManager(result.Conn)
	if err := sm.EnsureAgentInboxStream(ctx, 0); err != nil {
		return fmt.Errorf("create agent inbox stream: %w", err)
	}
	logger.Info("agent inbox stream ready", "name", hub.AgentInboxStream)

	// Pre-create durable pull consumers for all configured agents.
	agentNames := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		agentNames[i] = a.Name
	}
	if err := sm.EnsureAgentConsumers(ctx, agentNames); err != nil {
		return fmt.Errorf("create agent consumers: %w", err)
	}
	logger.Info("agent consumers ready", "count", len(agentNames))

	selfBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	reg := hub.NewRegistry(logger)
	if err := reg.Start(ctx, result.Conn); err != nil {
		return fmt.Errorf("start registry: %w", err)
	}

	// Create a messenger from the NATS connection for log publishing.
	messenger := &natsMessengerAdapter{conn: result.Conn}

	sup := hub.NewSupervisorWithMessenger(cfg, result.Conn.ConnectedUrl(), selfBin, "", result.ACL.AgentTokens, logger, noColor, messenger)
	if err := sup.Start(ctx); err != nil {
		return fmt.Errorf("start supervisor: %w", err)
	}

	mgmtHandler := hub.NewManagementHandler(
		result.Conn,
		sup,
		reg,
		config.ZlawHome(),
		logger,
	)

	// Start the hub management NATS inbox handler.
	go func() {
		if err := mgmtHandler.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Error("hub management handler stopped unexpectedly", "err", err)
		}
	}()

	// Start the agent-tool NATS inbox handler (HubInbox).
	hubInbox := hub.NewHubInbox(sup, reg, hubConfigAdapter{cfg}, logger)
	go func() {
		if err := mgmtHandler.StartToolInbox(ctx, hubInbox); err != nil && ctx.Err() == nil {
			logger.Error("hub tool inbox handler stopped unexpectedly", "err", err)
		}
	}()

	// Start the control socket for CLI access.
	controlPath := hub.ControlSocketPath(config.ZlawHome())
	ctrlSock := hub.NewControlSocket(
		controlPath,
		sup,
		reg,
		mgmtHandler,
		cfg,
		logger,
	)
	if err := ctrlSock.Start(ctx); err != nil {
		return fmt.Errorf("start control socket: %w", err)
	}

	// Start the web UI if an address was provided.
	var webUI *web.Server
	if dashboardAddr != "" {
		ws := hubWebState{
			cfg:       cfg,
			natsAddr:  result.Conn.ConnectedUrl(),
			sup:       sup,
			reg:       reg,
			auditPath: cfg.Hub.AuditLogPath,
		}
		webUI = web.NewServer(dashboardAddr, ws, logger)
		if err := webUI.Start(ctx); err != nil {
			return fmt.Errorf("start web: %w", err)
		}
	}

	logger.Info("hub started",
		"name", cfg.Hub.Name,
		"agents", len(cfg.Agents),
		"control", controlPath,
		"web", dashboardAddr,
	)

	// Watch zlaw.toml for agent list changes.
	go watchAgentList(ctx, configPath, sup, reg, sm, result.Conn, selfBin, result.ACL.AgentTokens, logger)

	// Block until context is cancelled (signal or parent shutdown).
	<-ctx.Done()
	logger.Info("hub shutting down")
	ctrlSock.Stop() //nolint:errcheck
	if webUI != nil {
		webUI.Stop(ctx) //nolint:errcheck
	}
	return nil
}

// hubWebState adapts hub components to [web.State].
type hubWebState struct {
	cfg       config.HubConfig
	natsAddr  string
	sup       *hub.Supervisor
	reg       *hub.Registry
	auditPath string
}

func (s hubWebState) HubConfig() config.HubConfig { return s.cfg }
func (s hubWebState) NATSAddr() string            { return s.natsAddr }
func (s hubWebState) Agents() []web.AgentInfo {
	entries := s.reg.List()
	statuses := s.sup.Statuses()
	statusMap := make(map[string]hub.AgentStatus, len(statuses))
	for _, st := range statuses {
		statusMap[st.Name] = st
	}
	out := make([]web.AgentInfo, 0, len(entries))
	for _, entry := range entries {
		info := web.AgentInfo{RegistryEntry: entry}
		if st, ok := statusMap[entry.Name]; ok {
			info.PID = st.PID
			info.Running = st.Running
			if st.LastErr != nil {
				info.LastErr = st.LastErr.Error()
			}
		}
		out = append(out, info)
	}
	return out
}

func (s hubWebState) AuditEntries(limit int, eventType string) ([]hub.AuditEntry, error) {
	return hub.ReadAuditLog(s.auditPath, limit, eventType)
}

type hubConfigAdapter struct{ cfg config.HubConfig }

func (a hubConfigAdapter) HubName() string { return a.cfg.Hub.Name }

// HubStatus prints the current hub status via the control socket.
// If the hub is not running (socket unreachable), it prints a note.
func HubStatus(ctx context.Context, jsonOutput bool) error {
	status, err := hubStatusFromSocket(ctx)
	if err != nil {
		// Hub not running.
		fmt.Println("Hub status: not running")
		fmt.Println("(Start the hub with 'zlaw hub start' to see status)")
		return nil //nolint:nilerr
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}
	return printHubStatus(status)
}

// hubStatusSocketResponse is the JSON payload from hub.status.
type hubStatusSocketResponse struct {
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	AgentCount int           `json:"agent_count"`
	Agents     []agentInfoV2 `json:"agents"`
	ConnStatus string        `json:"connected_status"`
	NATS       *natsInfo     `json:"nats,omitempty"`
}

type agentInfoV2 struct {
	Name          string   `json:"name"`
	Running       bool     `json:"running"`
	PID           int      `json:"pid"`
	LastErr       string   `json:"last_err,omitempty"`
	ConnStatus    string   `json:"conn_status"`
	LastHeartbeat string   `json:"last_heartbeat,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Roles         []string `json:"roles,omitempty"`
}

type natsInfo struct {
	Listen    string `json:"listen"`
	JetStream bool   `json:"jetstream"`
}

const socketDialTimeout = 2 * time.Second

// hubStatusFromSocket connects to the hub's control socket and requests hub.status.
func hubStatusFromSocket(ctx context.Context) (*hubStatusSocketResponse, error) {
	socketPath := hub.ControlSocketPath(config.ZlawHome())

	conn, err := net.DialTimeout("unix", socketPath, socketDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to hub control socket: %w", err)
	}
	defer func() { conn.Close() }() //nolint:errcheck

	req := `{"method":"hub.status"}` + "\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return nil, fmt.Errorf("send hub.status request: %w", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	var raw json.RawMessage
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read hub.status response: %w", err)
	}

	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse hub.status response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("hub.status error: %s", resp.Error)
	}

	var status hubStatusSocketResponse
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return nil, fmt.Errorf("decode hub.status result: %w", err)
	}
	return &status, nil
}

// printHubStatus prints the hub status in human-readable format.
func printHubStatus(s *hubStatusSocketResponse) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "Hub name:\t%s\n", s.Name)
	fmt.Fprintf(tw, "Status:\t\t%s\n", s.ConnStatus)
	if s.NATS != nil {
		fmt.Fprintf(tw, "NATS listen:\t%s\n", s.NATS.Listen)
		fmt.Fprintf(tw, "JetStream:\t%v\n", s.NATS.JetStream)
	}
	fmt.Fprintf(tw, "Agents:\t\t%d\n", s.AgentCount)
	tw.Flush() //nolint:errcheck

	if len(s.Agents) > 0 {
		fmt.Fprintln(os.Stdout, "\nAgent status:")
		tw2 := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw2, "Name\tRunning\tPID\tConn\tHeartbeat\tError")
		for _, a := range s.Agents {
			running := "yes"
			if !a.Running {
				running = "no"
			}
			heartbeat := a.LastHeartbeat
			if heartbeat == "" {
				heartbeat = "-"
			}
			fmt.Fprintf(tw2, "%s\t%s\t%d\t%s\t%s\t%s\n",
				a.Name, running, a.PID, a.ConnStatus, heartbeat, a.LastErr)
		}
		tw2.Flush() //nolint:errcheck
	}
	return nil
}

// setupHubLogger wraps the logger with hub prefix and color.
func setupHubLogger(logger *slog.Logger, noColor bool) *slog.Logger {
	opts := logging.Options{
		Label:   "[hub]",
		Color:   logging.ColorGray,
		NoColor: noColor,
		Time:    logging.DetectTimeFormat(),
	}
	h := logging.NewPrettyHandler(os.Stderr, opts)
	return slog.New(h)
}

// natsMessengerAdapter adapts *nats.Conn to messaging.Messenger interface.
type natsMessengerAdapter struct {
	conn *nats.Conn
}

func (a *natsMessengerAdapter) Publish(_ context.Context, subject string, payload []byte) error {
	return a.conn.Publish(subject, payload)
}

func (a *natsMessengerAdapter) Subscribe(_ context.Context, subject string, handler func([]byte)) (messaging.Subscription, error) {
	sub, err := a.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}

func (a *natsMessengerAdapter) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	msg, err := a.conn.RequestWithContext(ctx, subject, payload)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

func (a *natsMessengerAdapter) JetStream() messaging.JetStreamer {
	return nil // Hub doesn't need JetStream for log publishing
}

type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

var _ messaging.Messenger = (*natsMessengerAdapter)(nil)

// watchAgentList watches zlaw.toml for changes and updates the supervised
// agent set without requiring a hub restart. It runs until ctx is cancelled.
func watchAgentList(
	ctx context.Context,
	configPath string,
	sup *hub.Supervisor,
	reg *hub.Registry,
	sm *hub.StreamManager,
	natsConn *nats.Conn,
	selfBin string,
	agentTokens hub.AgentTokens,
	logger *slog.Logger,
) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("zlaw.toml watcher: create failed", "err", err)
		return
	}
	defer watcher.Close() //nolint:errcheck

	// Watch the directory so we catch writes to zlaw.toml.
	dir := filepath.Dir(configPath)
	if err := watcher.Add(dir); err != nil {
		logger.Warn("zlaw.toml watcher: add dir failed", "dir", dir, "err", err)
		return
	}
	logger.Info("zlaw.toml watcher started", "dir", dir)

	// debounceTicks prevents rapid successive events from reloading too often.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var pending bool

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only care about writes to the specific config file.
			if filepath.Base(event.Name) != filepath.Base(configPath) {
				continue
			}
			if event.Op&fsnotify.Write == 0 {
				continue
			}
			pending = true
		case <-ticker.C:
			if !pending {
				continue
			}
			pending = false
			reloadConfig(ctx, configPath, sup, reg, sm, natsConn, selfBin, agentTokens, logger)
		}
	}
}

// reloadConfig reloads zlaw.toml and syncs the supervised agent set:
// - New agents are spawned (unless disabled in agent.toml).
// - Removed agents are stopped and deregistered.
func reloadConfig(
	ctx context.Context,
	configPath string,
	sup *hub.Supervisor,
	reg *hub.Registry,
	sm *hub.StreamManager,
	natsConn *nats.Conn,
	selfBin string,
	agentTokens hub.AgentTokens,
	logger *slog.Logger,
) {
	newCfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		logger.Warn("zlaw.toml watcher: reload failed", "err", err)
		return
	}

	// Get current supervised names.
	current := sup.Statuses()
	currentNames := make(map[string]bool, len(current))
	for _, s := range current {
		currentNames[s.Name] = true
	}

	// Get new names from config.
	newNames := make(map[string]bool, len(newCfg.Agents))
	for _, a := range newCfg.Agents {
		newNames[a.Name] = true
	}

	// Remove agents that are no longer in the config.
	for _, name := range sortedKeys(currentNames) {
		if !newNames[name] {
			logger.Info("zlaw.toml watcher: removing agent", "name", name)
			if err := sup.Remove(name); err != nil {
				logger.Warn("zlaw.toml watcher: remove failed", "name", name, "err", err)
			}
			reg.Deregister(name)
		}
	}

	// Add new agents from the config.
	for _, entry := range newCfg.Agents {
		if currentNames[entry.Name] {
			continue // already supervised
		}

		// Skip disabled agents.
		if agentDisabled(entry.Dir) {
			logger.Info("zlaw.toml watcher: skipping disabled agent", "name", entry.Name)
			continue
		}

		logger.Info("zlaw.toml watcher: spawning agent", "name", entry.Name)
		// Ensure JetStream consumer exists.
		if err := sm.EnsureAgentConsumers(ctx, []string{entry.Name}); err != nil {
			logger.Warn("zlaw.toml watcher: create consumer failed", "name", entry.Name, "err", err)
		}
		if err := sup.Spawn(ctx, entry); err != nil {
			logger.Warn("zlaw.toml watcher: spawn failed", "name", entry.Name, "err", err)
		}
	}
}

// agentDisabled returns true if the agent's agent.toml has disabled = true.
func agentDisabled(agentDir string) bool {
	if agentDir == "" {
		return false
	}
	if !filepath.IsAbs(agentDir) {
		agentDir = filepath.Join(config.ZlawHome(), agentDir)
	}
	path := filepath.Join(agentDir, "agent.toml")
	var raw map[string]any
	_, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return false
	}
	if v, ok := raw["agent"].(map[string]any); ok {
		if b, ok := v["disabled"].(bool); ok {
			return b
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// deterministic order for tests
	for i := 0; i < len(keys)-1; i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
