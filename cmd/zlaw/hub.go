package main

import (
	"context"
	"log/slog"

	"github.com/chickenzord/zlaw/internal/app"
	"github.com/chickenzord/zlaw/internal/config"
)

type HubCmd struct {
	Start  HubStartCmd  `cmd:"" help:"start the hub (stub — Phase 2)"`
	Status HubStatusCmd `cmd:"" help:"show hub status (stub — Phase 2)"`
}

// ── hub start ─────────────────────────────────────────────────────────────────

type HubStartCmd struct {
	Config string `help:"path to zlaw.toml"`
}

func (c *HubStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	configPath := c.Config
	if configPath == "" {
		configPath = config.DefaultHubConfigPath()
	}
	return app.StartHub(ctx, configPath, logger)
}

// ── hub status ────────────────────────────────────────────────────────────────

type HubStatusCmd struct{}

func (c *HubStatusCmd) Run() error {
	return app.HubStatus()
}
