package main

import (
	"fmt"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

// AgentCmd keeps only dev-time commands that don't need a running hub:
// run, serve, attach, logs, auth
type AgentCmd struct {
	Run    AgentRunCmd    `cmd:"" help:"start the agent (interactive or stdin)"`
	Serve  AgentServeCmd  `cmd:"" help:"start the agent in daemon mode"`
	Attach AgentAttachCmd `cmd:"" help:"attach a terminal to a running daemon"`
	Logs   AgentLogsCmd   `cmd:"" help:"stream agent logs in pretty format"`
	Auth   AgentAuthCmd   `cmd:"" help:"manage agent credentials"`
}

// AgentFlags holds shared flags for agent-related commands.
type AgentFlags struct {
	Agent     string `short:"a" env:"ZLAW_AGENT"     help:"agent id; resolves to $ZLAW_HOME/agents/<id>"`
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
	if h := config.AgentHome(); h != "" {
		return filepath.Join(h, "workspace")
	}
	return ""
}
