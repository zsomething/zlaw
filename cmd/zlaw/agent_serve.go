package main

import (
	"context"
	"log/slog"

	"github.com/zsomething/zlaw/internal/app"
)

type AgentServeCmd struct {
	AgentFlags
}

func (c *AgentServeCmd) Run(ctx context.Context, logger *slog.Logger) error {
	agentDir, err := c.AgentFlags.resolveDir()
	if err != nil {
		return err
	}
	return app.ServeAgent(ctx, agentDir, logger)
}
