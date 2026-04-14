package main

import (
	"context"
	"log/slog"

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

type HubCmd struct {
	Start  HubStartCmd  `cmd:"" help:"start the hub"`
	Status HubStatusCmd `cmd:"" help:"show hub status"`
}

// ── hub start ─────────────────────────────────────────────────────────────────

type HubStartCmd struct {
	Config  string `help:"path to zlaw.toml"`
	NatsURL string `name:"nats-url" help:"connect to an external NATS server instead of embedding one"`
	NoColor bool   `name:"no-color" help:"disable ANSI color output"`
}

func (c *HubStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	configPath := c.Config
	if configPath == "" {
		configPath = config.DefaultHubConfigPath()
	}
	if c.NoColor || hub.DefaultNoColor() {
		return app.StartHub(ctx, configPath, c.NatsURL, logger, true)
	}
	return app.StartHub(ctx, configPath, c.NatsURL, logger, false)
}

// ── hub status ────────────────────────────────────────────────────────────────

type HubStatusCmd struct{}

func (c *HubStatusCmd) Run() error {
	return app.HubStatus()
}
