package main

import (
	"context"
	"log/slog"

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/config"
)

type HubCmd struct {
	Start  HubStartCmd  `cmd:"" help:"start the hub"`
	Status HubStatusCmd `cmd:"" help:"show hub status"`
}

// ── hub start ─────────────────────────────────────────────────────────────────

type HubStartCmd struct {
	Config  string `help:"path to zlaw.toml"`
	NatsURL string `name:"nats-url" help:"connect to an external NATS server instead of embedding one"`
}

func (c *HubStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	configPath := c.Config
	if configPath == "" {
		configPath = config.DefaultHubConfigPath()
	}
	return app.StartHub(ctx, configPath, c.NatsURL, logger)
}

// ── hub status ────────────────────────────────────────────────────────────────

type HubStatusCmd struct{}

func (c *HubStatusCmd) Run() error {
	return app.HubStatus()
}
