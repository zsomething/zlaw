package main

import (
	"context"
	"log/slog"

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

type HubCmd struct {
	Start   HubStartCmd   `cmd:"" help:"start the hub in the background (daemon mode)"`
	Run     HubRunCmd     `cmd:"" help:"run the hub in the foreground (blocking)"`
	Stop    HubStopCmd    `cmd:"" help:"stop a running hub"`
	Restart HubRestartCmd `cmd:"" help:"restart the hub (stop then start)"`
	Status  HubStatusCmd  `cmd:"" help:"show hub status"`
}

// ── hub start ─────────────────────────────────────────────────────────────────

// HubStartCmd starts the hub in the background (daemon mode).
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
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return err
	}
	noColor := c.NoColor || hub.DefaultNoColor()
	webAddr := resolveWebAddr(cfg)
	return app.StartHub(ctx, configPath, c.NatsURL, logger, noColor, webAddr)
}

// ── hub run ─────────────────────────────────────────────────────────────────

// HubRunCmd runs the hub in the foreground (blocking). Equivalent to the
// current "hub start" behavior — kept for clarity.
type HubRunCmd struct {
	Config  string `help:"path to zlaw.toml"`
	NatsURL string `name:"nats-url" help:"connect to an external NATS server instead of embedding one"`
	NoColor bool   `name:"no-color" help:"disable ANSI color output"`
}

func (c *HubRunCmd) Run(ctx context.Context, logger *slog.Logger) error {
	configPath := c.Config
	if configPath == "" {
		configPath = config.DefaultHubConfigPath()
	}
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return err
	}
	noColor := c.NoColor || hub.DefaultNoColor()
	webAddr := resolveWebAddr(cfg)
	return app.RunHub(ctx, configPath, c.NatsURL, logger, noColor, webAddr)
}

// ── hub stop ─────────────────────────────────────────────────────────────────

type HubStopCmd struct{}

func (c *HubStopCmd) Run() error {
	return app.StopHub()
}

// ── hub restart ───────────────────────────────────────────────────────────────

type HubRestartCmd struct {
	Config  string `help:"path to zlaw.toml"`
	NatsURL string `name:"nats-url" help:"connect to an external NATS server instead of embedding one"`
	NoColor bool   `name:"no-color" help:"disable ANSI color output"`
}

func (c *HubRestartCmd) Run(ctx context.Context, logger *slog.Logger) error {
	if err := app.StopHub(); err != nil {
		return err
	}
	configPath := c.Config
	if configPath == "" {
		configPath = config.DefaultHubConfigPath()
	}
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return err
	}
	noColor := c.NoColor || hub.DefaultNoColor()
	webAddr := resolveWebAddr(cfg)
	return app.StartHub(ctx, configPath, c.NatsURL, logger, noColor, webAddr)
}

// ── hub status ────────────────────────────────────────────────────────────────

type HubStatusCmd struct {
	JSON bool `long:"json" help:"output status as JSON"`
}

func (c *HubStatusCmd) Run(ctx context.Context, _ *slog.Logger) error {
	return app.HubStatus(ctx, c.JSON)
}

// ── helpers ─────────────────────────────────────────────────────────────────

// resolveWebAddr returns the web UI bind address from config, or "" if not enabled.
func resolveWebAddr(cfg config.HubConfig) string {
	if !cfg.Web.Enabled {
		return ""
	}
	if cfg.Web.BindAddress != "" {
		return cfg.Web.BindAddress
	}
	return "127.0.0.1:7420"
}
