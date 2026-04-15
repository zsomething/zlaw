package main

import (
	"fmt"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

type AgentCmd struct {
	Run    AgentRunCmd    `cmd:"" help:"start the agent (interactive or stdin)"`
	Serve  AgentServeCmd  `cmd:"" help:"start the agent in daemon mode"`
	Attach AgentAttachCmd `cmd:"" help:"attach a terminal to a running daemon"`
	Logs   AgentLogsCmd   `cmd:"" help:"stream agent logs in pretty format"`
}

// AgentFlags are embedded by commands that need to resolve an agent directory.
type AgentFlags struct {
	Agent    string `short:"a" env:"ZLAW_AGENT"     help:"agent name; resolves to $ZLAW_HOME/agents/<name>"`
	AgentDir string `          env:"ZLAW_AGENT_DIR" help:"explicit path to agent directory (overrides --agent)"`
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
