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

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

type AgentCmd struct {
	Run     AgentRunCmd     `cmd:"" help:"start the agent (interactive or stdin)"`
	Serve   AgentServeCmd   `cmd:"" help:"start the agent in daemon mode"`
	Attach  AgentAttachCmd  `cmd:"" help:"attach a terminal to a running daemon"`
	Logs    AgentLogsCmd    `cmd:"" help:"stream agent logs in pretty format"`
	List    AgentListCmd    `cmd:"" help:"list all agents managed by the hub"`
	Status  AgentStatusCmd  `cmd:"" help:"show status of a specific agent"`
	Stop    AgentStopCmd    `cmd:"" help:"stop a running agent"`
	Restart AgentRestartCmd `cmd:"" help:"restart a stopped or running agent"`
}

// AgentFlags are embedded by commands that need to resolve an agent directory.
type AgentFlags struct {
	Agent     string `short:"a" env:"ZLAW_AGENT"     help:"agent name; resolves to $ZLAW_HOME/agents/<name>"`
	AgentDir  string `          env:"ZLAW_AGENT_DIR" help:"explicit path to agent directory (overrides --agent)"`
	Workspace string `          env:"ZLAW_WORKSPACE" help:"path to agent workspace (SOUL.md, IDENTITY.md); defaults to $ZLAW_HOME/workspaces/<name>"`
}

func (f AgentFlags) resolveDir() (string, error) {
	if f.AgentDir != "" {
		return f.AgentDir, nil
	}
	if f.Agent != "" {
		return filepath.Join(config.ZlawHome(), "agents", f.Agent), nil
	}
	return "", fmt.Errorf("--agent <name> or --agent-dir <path> is required (or set ZLAW_AGENT / ZLAW_AGENT_DIR)")
}

func (f AgentFlags) resolveWorkspace() string {
	if f.Workspace != "" {
		return f.Workspace
	}
	if f.Agent != "" {
		return filepath.Join(config.ZlawHome(), "workspaces", f.Agent)
	}
	return ""
}

// ── agent list ────────────────────────────────────────────────────────────────

type AgentListCmd struct {
	JSON bool `long:"json" help:"output as JSON"`
}

func (c *AgentListCmd) Run(ctx context.Context, _ *slog.Logger) error {
	entries, err := agentListFromSocket(ctx)
	if err != nil {
		return err
	}

	if c.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tConn\tLastHeartbeat\tRoles")
	for _, e := range entries {
		heartbeat := e.LastHeartbeat
		if heartbeat == "" {
			heartbeat = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%v\n", e.Name, e.Status, heartbeat, e.Roles)
	}
	return tw.Flush()
}

// ── agent status ──────────────────────────────────────────────────────────────

type AgentStatusCmd struct {
	Name string `arg:"true" help:"agent name"`
	JSON bool   `long:"json" help:"output as JSON"`
}

func (c *AgentStatusCmd) Run(ctx context.Context, _ *slog.Logger) error {
	status, err := agentStatusFromSocket(ctx, c.Name)
	if err != nil {
		return err
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
	fmt.Printf("Name:     %s\n", status.Name)
	fmt.Printf("Running:  %s\n", running)
	if status.PID > 0 {
		fmt.Printf("PID:      %d\n", status.PID)
	}
	fmt.Printf("Conn:     %s\n", status.ConnStatus)
	if status.LastHeartbeat != "" {
		fmt.Printf("Heartbeat: %s\n", status.LastHeartbeat)
	}
	if len(status.Capabilities) > 0 {
		fmt.Printf("Capabilities: %v\n", status.Capabilities)
	}
	if len(status.Roles) > 0 {
		fmt.Printf("Roles: %v\n", status.Roles)
	}
	if status.LastErr != "" {
		fmt.Printf("Error:  %s\n", status.LastErr)
	}
	return nil
}

// ── agent stop ─────────────────────────────────────────────────────────────────

type AgentStopCmd struct {
	Name string `arg:"true" help:"agent name"`
}

func (c *AgentStopCmd) Run(ctx context.Context, _ *slog.Logger) error {
	return agentAction(ctx, "agent.stop", map[string]any{"name": c.Name})
}

// ── agent restart ─────────────────────────────────────────────────────────────

type AgentRestartCmd struct {
	Name string `arg:"true" help:"agent name"`
}

func (c *AgentRestartCmd) Run(ctx context.Context, _ *slog.Logger) error {
	return agentAction(ctx, "agent.restart", map[string]any{"name": c.Name})
}

// ── Socket helpers ────────────────────────────────────────────────────────────

func socketConn() (net.Conn, error) {
	socketPath := hub.ControlSocketPath(config.ZlawHome())
	return net.DialTimeout("unix", socketPath, 2*time.Second)
}

func agentListFromSocket(ctx context.Context) ([]agentListEntry, error) {
	conn, err := socketConn()
	if err != nil {
		return nil, fmt.Errorf("connect to hub (is it running?): %w", err)
	}
	defer func() { conn.Close() }() //nolint:errcheck

	req := map[string]any{"method": "agent.list"}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("agent.list error: %s", resp.Error)
	}
	var inner struct {
		Agents []agentListEntry `json:"agents"`
	}
	if err := json.Unmarshal(resp.Result, &inner); err != nil {
		return nil, fmt.Errorf("decode result: %w", err)
	}
	return inner.Agents, nil
}

func agentStatusFromSocket(ctx context.Context, name string) (agentStatusEntry, error) {
	conn, err := socketConn()
	if err != nil {
		return agentStatusEntry{}, fmt.Errorf("connect to hub (is it running?): %w", err)
	}
	defer func() { conn.Close() }() //nolint:errcheck

	req := map[string]any{"method": "agent.status", "params": map[string]any{"name": name}}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
	conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return agentStatusEntry{}, fmt.Errorf("send request: %w", err)
	}

	var raw json.RawMessage
	if err := json.NewDecoder(conn).Decode(&raw); err != nil {
		return agentStatusEntry{}, fmt.Errorf("read response: %w", err)
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return agentStatusEntry{}, fmt.Errorf("parse response: %w", err)
	}
	if !resp.OK {
		return agentStatusEntry{}, fmt.Errorf("agent.status error: %s", resp.Error)
	}
	var status agentStatusEntry
	if err := json.Unmarshal(resp.Result, &status); err != nil {
		return agentStatusEntry{}, fmt.Errorf("decode result: %w", err)
	}
	return status, nil
}

func agentAction(ctx context.Context, method string, params map[string]any) error {
	conn, err := socketConn()
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

type agentListEntry struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	Roles         []string `json:"roles"`
	Status        string   `json:"status"`
	LastHeartbeat string   `json:"last_heartbeat"`
}

type agentStatusEntry struct {
	Name          string   `json:"name"`
	Running       bool     `json:"running"`
	PID           int      `json:"pid"`
	LastErr       string   `json:"last_err,omitempty"`
	ConnStatus    string   `json:"conn_status"`
	LastHeartbeat string   `json:"last_heartbeat,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	Roles         []string `json:"roles,omitempty"`
}
