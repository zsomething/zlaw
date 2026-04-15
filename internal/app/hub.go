package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/logging"
	"github.com/zsomething/zlaw/internal/messaging"
)

// StartHub loads the hub config and starts the hub process.
func StartHub(ctx context.Context, configPath string, externalNATSURL string, logger *slog.Logger, noColor bool) error {
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

	// Create the durable agent inbox stream if JetStream is enabled.
	if result.JetStream != nil {
		sm := hub.NewStreamManager(result.Conn)
		if err := sm.EnsureAgentInboxStream(ctx, 0); err != nil {
			return fmt.Errorf("create agent inbox stream: %w", err)
		}
		logger.Info("agent inbox stream ready", "name", hub.AgentInboxStream)
	}

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

	managerName := managerAgentName(cfg)
	mgmtHandler := hub.NewManagementHandler(
		result.Conn,
		sup,
		reg,
		managerName,
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

	logger.Info("hub started",
		"name", cfg.Hub.Name,
		"agents", len(cfg.Agents),
		"manager", managerName,
		"control", controlPath,
	)

	// Block until context is cancelled (signal or parent shutdown).
	<-ctx.Done()
	logger.Info("hub shutting down")
	ctrlSock.Stop() //nolint:errcheck
	return nil
}

// hubConfigAdapter adapts config.HubConfig to hub.ToolHubConfig.
type hubConfigAdapter struct{ cfg config.HubConfig }

func (a hubConfigAdapter) HubName() string { return a.cfg.Hub.Name }

// managerAgentName returns the name of the first agent entry marked Manager,
// or empty string if none.
func managerAgentName(cfg config.HubConfig) string {
	for _, a := range cfg.Agents {
		if a.Manager {
			return a.Name
		}
	}
	return ""
}

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
