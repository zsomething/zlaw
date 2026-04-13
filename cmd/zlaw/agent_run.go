package main

import (
	"context"
	"log/slog"

	"github.com/zsomething/zlaw/internal/app"
)

type AgentRunCmd struct {
	AgentFlags
	Session   string `default:"default" help:"session identifier"`
	Verbose   bool   `short:"v"         help:"show thinking and tool calls"`
	ShowUsage bool   `name:"show-usage" help:"print token usage after each turn (per-turn and cumulative)"`
}

func (c *AgentRunCmd) Run(ctx context.Context, logger *slog.Logger) error {
	agentDir, err := c.AgentFlags.resolveDir()
	if err != nil {
		return err
	}
	return app.RunAgent(ctx, agentDir, app.AgentRunOptions{
		Session:   c.Session,
		Verbose:   c.Verbose,
		ShowUsage: c.ShowUsage,
	}, logger)
}
