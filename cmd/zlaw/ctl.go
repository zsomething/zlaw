package main

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

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/executor"
	"github.com/zsomething/zlaw/internal/hub"
)

// Types shared between ctl and agent commands

type agentListEntry struct {
	ID            string   `json:"id"`
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	Roles         []string `json:"roles"`
	Status        string   `json:"status"`
	LastHeartbeat string   `json:"last_heartbeat"`
}

type agentStatusEntry struct {
	ID            string   `json:"id"`
	Running       bool     `json:"running"`
	PID           int      `json:"pid"`
	LastErr       string   `json:"last_err,omitempty"`
	ConnStatus    string   `json:"conn_status"`
	LastHeartbeat string   `json:"last_heartbeat,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Roles         []string `json:"roles,omitempty"`
}

// ── Templates ────────────────────────────────────────────────────────────────

// agentTOMLTemplate has agent name, executor, and target substituted for %s, %s, %s.
const agentTOMLTemplate = `[agent]
id = %q
executor = %q
target = %q
restart_policy = "on-failure"
description = ""

[llm]
backend = "anthropic"
model = "claude-sonnet-4-5"
auth_profile = "anthropic"
max_tokens = 4096
timeout_sec = 60

[tools]
allowed = []

# Uncomment and configure adapters after setting up credentials:
# [[adapter]]
# type = "telegram"
# auth_profile = "telegram"
`

const credentialsTOMLTemplate = `[profiles.anthropic]
name = "anthropic"
data = { api_key = "${ANTHROPIC_API_KEY}" }

[profiles.telegram]
name = "telegram"
data = { telegram_bot_token = "${TELEGRAM_BOT_TOKEN}" }

[profiles.fizzy]
name = "fizzy"
data = { fizzy_api_key = "${FIZZY_API_KEY}" }
`

const ctlSoulMDTemplate = `You are a helpful personal assistant.
`

// ctlIdentityMDTemplate has agent name substituted for %s.
const ctlIdentityMDTemplate = `# Identity

Your name is %s.
`

// ── CtlCmd ───────────────────────────────────────────────────────────────────

type CtlCmd struct {
	Start     CtlStartCmd     `cmd:"" help:"start NATS, hub, and all agents"`
	Stop      CtlStopCmd      `cmd:"" help:"stop NATS, hub, and all agents"`
	Create    CtlCreateCmd    `cmd:"" help:"create and register a new agent"`
	Get       CtlGetCmd       `cmd:"" help:"get resource info"`
	Agent     CtlAgentCmd     `cmd:"" help:"agent lifecycle management"`
	Configure CtlConfigureCmd `cmd:"" help:"update a runtime field"`
	Logs      CtlLogsCmd      `cmd:"" help:"stream agent logs"`
}

// ── ctl agent ─────────────────────────────────────────────────────────────────

type CtlAgentCmd struct {
	Start   CtlAgentStartCmd   `cmd:"" help:"start an agent"`
	Stop    CtlAgentStopCmd    `cmd:"" help:"stop an agent"`
	Restart CtlAgentRestartCmd `cmd:"" help:"restart an agent"`
	Delete  CtlAgentDeleteCmd  `cmd:"" help:"delete an agent (preserves home)"`
}

type CtlAgentStartCmd struct {
	ID string `arg:"true" help:"agent id"`
}

type CtlAgentStopCmd struct {
	ID string `arg:"true" help:"agent id"`
}

type CtlAgentRestartCmd struct {
	ID string `arg:"true" help:"agent id"`
}

type CtlAgentDeleteCmd struct {
	ID    string `arg:"true" help:"agent id"`
	Prune bool   `short:"p" help:"also delete agent home directory"`
}

// ── ctl get ──────────────────────────────────────────────────────────────────

type CtlGetCmd struct {
	Agents CtlGetAgentsCmd `cmd:"" help:"list all agents"`
	Agent  CtlGetAgentCmd  `cmd:"" help:"show agent detail"`
	Hub    CtlGetHubCmd    `cmd:"" help:"show hub status"`
}

// ctl get agents

type CtlGetAgentsCmd struct {
	JSON bool `long:"json" help:"output as JSON"`
}

func (c *CtlGetAgentsCmd) Run(ctx context.Context, _ *slog.Logger) error {
	conn, err := ctlSocketConn()
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	req := map[string]any{"method": "agent.list"}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("agent.list error: %s", resp.Error)
	}
	var inner struct {
		Agents []agentListEntry `json:"agents"`
	}
	if err := json.Unmarshal(resp.Result, &inner); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}

	if c.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(inner.Agents)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tConn\tLastHeartbeat\tRoles")
	for _, e := range inner.Agents {
		heartbeat := e.LastHeartbeat
		if heartbeat == "" {
			heartbeat = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%v\n", e.ID, e.Status, heartbeat, e.Roles)
	}
	return tw.Flush()
}

// ctl get agent <name>

type CtlGetAgentCmd struct {
	Name string `arg:"true" help:"agent id"`
	JSON bool   `long:"json" help:"output as JSON"`
}

func (c *CtlGetAgentCmd) Run(ctx context.Context, _ *slog.Logger) error {
	conn, err := ctlSocketConn()
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	req := map[string]any{"method": "agent.status", "params": map[string]any{"id": c.Name}}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("agent.status error: %s", resp.Error)
	}
	var status agentStatusEntry
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}

	if c.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	running := "yes"
	if !status.Running {
		running = "no"
	}
	fmt.Printf("ID:       %s\n", status.ID)
	fmt.Printf("Running:   %s\n", running)
	if status.PID > 0 {
		fmt.Printf("PID:       %d\n", status.PID)
	}
	fmt.Printf("Conn:      %s\n", status.ConnStatus)
	if status.LastHeartbeat != "" {
		fmt.Printf("Heartbeat: %s\n", status.LastHeartbeat)
	}
	if len(status.Capabilities) > 0 {
		fmt.Printf("Caps:      %v\n", status.Capabilities)
	}
	if len(status.Roles) > 0 {
		fmt.Printf("Roles:     %v\n", status.Roles)
	}
	if status.LastErr != "" {
		fmt.Printf("Error:     %s\n", status.LastErr)
	}
	return nil
}

// ctl get hub

type CtlGetHubCmd struct {
	JSON bool `long:"json" help:"output as JSON"`
}

func (c *CtlGetHubCmd) Run(ctx context.Context, _ *slog.Logger) error {
	conn, err := ctlSocketConn()
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	req := map[string]any{"method": "hub.status"}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("hub.status error: %s", resp.Error)
	}
	var status hubStatusDisplay
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}

	if c.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Hub:       %s\n", status.Name)
	if status.NATS != nil {
		fmt.Printf("NATS:      nats://%s\n", status.NATS.Listen)
		fmt.Printf("JetStream: %v\n", status.NATS.JetStream)
	}
	fmt.Printf("Agents:    %d\n", status.AgentCount)
	fmt.Printf("Status:    running\n")
	return nil
}

type hubStatusDisplay struct {
	Name       string      `json:"name"`
	AgentCount int         `json:"agent_count"`
	NATS       *natsStatus `json:"nats,omitempty"`
}

type natsStatus struct {
	Listen    string `json:"listen"`
	JetStream bool   `json:"jetstream"`
}

// ── ctl create ────────────────────────────────────────────────────────────────

type CtlCreateCmd struct {
	Agent CtlCreateAgentCmd `cmd:"" help:"create and register a new agent"`
}

type CtlCreateAgentCmd struct {
	Name      string `arg:"true" help:"agent id"`
	Executor  string `short:"e" name:"executor" help:"executor type (subprocess, systemd, docker)"`
	Target    string `short:"t" name:"target" help:"target (local, ssh)"`
	AgentHome string `name:"agent-home" help:"absolute path for agent home (default: $ZLAW_HOME/agents/<name>)"`
	Start     bool   `help:"spawn the agent after registration"`
}

func (c *CtlCreateAgentCmd) Run(ctx context.Context, _ *slog.Logger) error {
	agentHome := c.AgentHome
	if agentHome == "" {
		agentHome = filepath.Join(config.ZlawHome(), "agents", c.Name)
	}
	if !filepath.IsAbs(agentHome) {
		abs, err := filepath.Abs(agentHome)
		if err != nil {
			return fmt.Errorf("resolve agent-home: %w", err)
		}
		agentHome = abs
	}

	// Default executor/target
	executor := c.Executor
	if executor == "" {
		executor = "subprocess"
	}
	target := c.Target
	if target == "" {
		target = "local"
	}

	// 1. Create agent home directory.
	if err := os.MkdirAll(agentHome, 0o700); err != nil {
		return fmt.Errorf("create agent home %s: %w", agentHome, err)
	}

	// 2. Create workspace/ subdir.
	workspaceDir := filepath.Join(agentHome, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	// 3. Scaffold files (skip if already exist).
	agentToml := fmt.Sprintf(agentTOMLTemplate, c.Name, executor, target)
	type scaffold struct {
		path    string
		content string
		mode    os.FileMode
	}
	files := []scaffold{
		{filepath.Join(agentHome, "agent.toml"), agentToml, 0o600},
		{filepath.Join(agentHome, "credentials.toml"), credentialsTOMLTemplate, 0o600},
		{filepath.Join(agentHome, "SOUL.md"), ctlSoulMDTemplate, 0o644},
		{filepath.Join(agentHome, "IDENTITY.md"), fmt.Sprintf(ctlIdentityMDTemplate, c.Name), 0o644},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); os.IsNotExist(err) {
			if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
				return fmt.Errorf("write %s: %w", f.path, err)
			}
			fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
		} else {
			fmt.Fprintf(os.Stdout, "  exists   %s\n", f.path)
		}
	}

	// 4. Register with hub via control socket.
	method := "agent.create"
	params := map[string]any{
		"id":  c.Name,
		"dir": agentHome,
	}
	if err := ctlSocketCall(method, params); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register with hub: %v\n", err)
		fmt.Fprintf(os.Stderr, "  start the hub and run: zlaw ctl create agent %s\n", c.Name)
	} else {
		fmt.Fprintf(os.Stdout, "  registered agent %q with hub (dir: %s)\n", c.Name, agentHome)
	}

	fmt.Fprintf(os.Stdout, "\nAgent %q ready at %s\n", c.Name, agentHome)
	return nil
}

// ── ctl start ─────────────────────────────────────────────────────────────────

// CtlStartCmd starts the entire system (NATS, hub, all agents).
type CtlStartCmd struct {
	NoStartAgents bool `name:"no-start-agents" help:"start NATS and hub only, skip agent spawning"`
}

func (c *CtlStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	fmt.Printf("starting system...\n")

	// Start hub (which includes NATS)
	if err := app.StartHub(ctx, config.DefaultHubConfigPath(), "", "", logger, hub.DefaultNoColor()); err != nil {
		return fmt.Errorf("start hub: %w", err)
	}
	fmt.Printf("  hub started\n")

	// Start all agents if not skipped
	if !c.NoStartAgents {
		cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Get NATS URL from hub status
		natsURL := "nats://127.0.0.1:4222" // Default, should get from hub

		for _, agent := range cfg.Agents {
			if agent.Disabled {
				continue
			}

			// Get executor for this agent
			execType := agent.Executor
			if execType == "" {
				execType = "subprocess"
			}
			exec := executor.NewExecutor(execType)

			// Build agent config for executor
			agentCfg := executor.AgentConfig{
				ID:            agent.ID,
				Dir:           agent.Dir,
				Binary:        agent.Binary,
				Executor:      agent.Executor,
				Target:        agent.Target,
				TargetSSH:     agent.TargetSSH,
				RestartPolicy: string(agent.RestartPolicy),
				NATSURL:       natsURL,
				AuthProfiles:  agent.AuthProfiles,
			}

			if err := exec.Start(ctx, agentCfg); err != nil {
				logger.Warn("failed to start agent", "id", agent.ID, "err", err)
			} else {
				fmt.Printf("  agent %q started\n", agent.ID)
			}
		}
	}

	fmt.Printf("system started\n")
	return nil
}

// ── ctl stop ──────────────────────────────────────────────────────────────────

// CtlStopCmd stops the entire system (NATS, hub, all agents).
type CtlStopCmd struct{}

func (c *CtlStopCmd) Run(ctx context.Context, logger *slog.Logger) error {
	// Stop all agents first
	cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err == nil {
		for _, agent := range cfg.Agents {
			if err := ctlAgentAction("agent.stop", map[string]any{"id": agent.ID}); err != nil {
				logger.Warn("failed to stop agent", "id", agent.ID, "err", err)
			}
		}
	}

	// Stop hub via hub stop command
	if err := app.StopHub(""); err != nil {
		logger.Warn("failed to stop hub", "err", err)
	}

	fmt.Printf("system stopped\n")
	return nil
}

// ── ctl agent start ──────────────────────────────────────────────────────────────

func (c *CtlAgentStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	// Load agent config
	cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	agent, ok := cfg.FindAgent(c.ID)
	if !ok {
		return fmt.Errorf("agent %q not found", c.ID)
	}

	// Get executor
	execType := agent.Executor
	if execType == "" {
		execType = "subprocess"
	}
	exec := executor.NewExecutor(execType)

	// Build agent config
	agentCfg := executor.AgentConfig{
		ID:            agent.ID,
		Dir:           agent.Dir,
		Binary:        agent.Binary,
		Executor:      agent.Executor,
		Target:        agent.Target,
		TargetSSH:     agent.TargetSSH,
		RestartPolicy: string(agent.RestartPolicy),
		NATSURL:       "nats://127.0.0.1:4222",
		AuthProfiles:  agent.AuthProfiles,
	}

	if err := exec.Start(ctx, agentCfg); err != nil {
		return fmt.Errorf("start agent: %w", err)
	}
	fmt.Printf("agent %q started\n", c.ID)
	return nil
}

// ── ctl agent stop ────────────────────────────────────────────────────────────

func (c *CtlAgentStopCmd) Run(ctx context.Context, _ *slog.Logger) error {
	// Load agent config to get executor type
	cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	agent, ok := cfg.FindAgent(c.ID)
	if !ok {
		return fmt.Errorf("agent %q not found", c.ID)
	}

	// Get executor
	execType := agent.Executor
	if execType == "" {
		execType = "subprocess"
	}
	exec := executor.NewExecutor(execType)

	if err := exec.Stop(ctx, c.ID); err != nil {
		return fmt.Errorf("stop agent: %w", err)
	}
	fmt.Printf("agent %q stopped\n", c.ID)
	return nil
}

// ── ctl agent restart ──────────────────────────────────────────────────────────

func (c *CtlAgentRestartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	// Stop first
	stop := CtlAgentStopCmd{ID: c.ID}
	if err := stop.Run(ctx, logger); err != nil {
		return err
	}
	// Start again
	start := CtlAgentStartCmd{ID: c.ID}
	return start.Run(ctx, logger)
}

// ── ctl agent delete ────────────────────────────────────────────────────────────

func (c *CtlAgentDeleteCmd) Run(ctx context.Context, logger *slog.Logger) error {
	// Load agent config to get executor type
	cfg, err := config.LoadHubConfig(config.DefaultHubConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	agent, ok := cfg.FindAgent(c.ID)
	if !ok {
		return fmt.Errorf("agent %q not found", c.ID)
	}

	// Stop agent via executor
	execType := agent.Executor
	if execType == "" {
		execType = "subprocess"
	}
	exec := executor.NewExecutor(execType)
	_ = exec.Stop(ctx, c.ID)

	// Remove from zlaw.toml
	if err := cfg.RemoveAgent(c.ID); err != nil {
		return fmt.Errorf("remove agent from config: %w", err)
	}

	// Delete home directory if --prune
	if c.Prune {
		agentDir := filepath.Join(config.ZlawHome(), "agents", c.ID)
		if err := os.RemoveAll(agentDir); err != nil {
			return fmt.Errorf("delete agent home: %w", err)
		}
		fmt.Printf("agent %q deleted (home pruned)\n", c.ID)
	} else {
		fmt.Printf("agent %q deleted (home preserved)\n", c.ID)
	}
	return nil
}

// ── ctl configure ─────────────────────────────────────────────────────────────

type CtlConfigureCmd struct {
	Name  string `arg:"true" help:"agent id"`
	Key   string `arg:"true" help:"field key (e.g., llm.model, llm.backend)"`
	Value string `arg:"true" help:"field value"`
}

func (c *CtlConfigureCmd) Run(ctx context.Context, _ *slog.Logger) error {
	params := map[string]any{"id": c.Name, "key": c.Key, "value": c.Value}
	if err := ctlAgentAction("agent.configure", params); err != nil {
		return err
	}
	fmt.Printf("agent %q configured: %s = %s\n", c.Name, c.Key, c.Value)
	return nil
}

// ── ctl logs ────────────────────────────────────────────────────────────────────

type CtlLogsCmd struct {
	Agent    string        `help:"agent id to filter logs (default: all agents)"`
	Level    string        `help:"minimum log level (debug/info/warn/error)"`
	Since    time.Duration `help:"show logs from the last N seconds"`
	Follow   bool          `short:"f" help:"follow logs continuously"`
	NatsURL  string        `env:"ZLAW_NATS_URL" help:"NATS server URL"`
	NatsCred string        `env:"ZLAW_NATS_CREDS" help:"NATS credentials token"`
}

// ctl logs reuses AgentLogsCmd logic.
func (c *CtlLogsCmd) Run(ctx context.Context, logger *slog.Logger) error {
	// Delegate to the existing AgentLogsCmd for NATS streaming logic.
	cmd := AgentLogsCmd{
		Agent:    c.Agent,
		Level:    c.Level,
		Since:    c.Since,
		Follow:   c.Follow,
		NatsURL:  c.NatsURL,
		NatsCred: c.NatsCred,
	}
	return cmd.Run(ctx)
}

// ── Socket helpers ────────────────────────────────────────────────────────────

func ctlSocketConn() (net.Conn, error) {
	runDir := filepath.Join(config.ZlawHome(), "run")
	socketPath := hub.ControlSocketPath(runDir)
	return net.DialTimeout("unix", socketPath, 2*time.Second)
}

func ctlSocketCall(method string, params map[string]any) error {
	conn, err := ctlSocketConn()
	if err != nil {
		return fmt.Errorf("connect to hub: %w", err)
	}
	defer conn.Close() //nolint:errcheck

	req := map[string]any{"method": method, "params": params}
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	return nil
}

func ctlAgentAction(method string, params map[string]any) error {
	conn, err := ctlSocketConn()
	if err != nil {
		return fmt.Errorf("connect to hub (is it running?): %w", err)
	}
	defer func() { conn.Close() }() //nolint:errcheck

	req := map[string]any{"method": method, "params": params}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("%s error: %s", method, resp.Error)
	}
	return nil
}
