package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/app"
	"github.com/chickenzord/zlaw/internal/config"
)

type AgentAttachCmd struct {
	AgentFlags
	Session string `default:"default" help:"session ID to attach to"`
}

func (c *AgentAttachCmd) Run(ctx context.Context, logger *slog.Logger) error {
	name := c.Agent
	if name == "" {
		if c.AgentDir == "" {
			return fmt.Errorf("--agent <name> is required for attach (or set ZLAW_AGENT)")
		}
		name = filepath.Base(c.AgentDir)
	}
	_ = config.ZlawHome() // ensure env is loaded
	return app.AttachAgent(ctx, name, c.Session, logger)
}
