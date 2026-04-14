package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/logging"
)

type AgentRunCmd struct {
	AgentFlags
	Session   string `default:"default" help:"session identifier"`
	Verbose   bool   `short:"v"         help:"show thinking and tool calls"`
	ShowUsage bool   `name:"show-usage" help:"print token usage after each turn (per-turn and cumulative)"`
}

func (c *AgentRunCmd) Run(ctx context.Context, logger *slog.Logger) error {
	agentDir, err := c.resolveDir()
	if err != nil {
		return err
	}
	workspace := c.resolveWorkspace()

	// Wrap logger with agent label for PrettyHandler mode.
	name := os.Getenv("ZLAW_AGENT")
	if name == "" {
		name = c.Agent
	}
	if name != "" && logging.DetectFormat() != logging.LogFormatJSON {
		label := "[agent:" + name + "]"
		color := logging.AgentColor(name)
		opts := logging.Options{
			Label:   label,
			Color:   color,
			NoColor: logging.DetectNoColor(),
			Time:    logging.DetectTimeFormat(),
		}
		logger = logging.LoggerWithOptions(opts)
	}

	return app.RunAgent(ctx, agentDir, workspace, app.AgentRunOptions{
		Session:   c.Session,
		Verbose:   c.Verbose,
		ShowUsage: c.ShowUsage,
	}, logger)
}
