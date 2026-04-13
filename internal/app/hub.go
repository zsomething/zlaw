package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

// StartHub loads the hub config and starts the hub process.
func StartHub(ctx context.Context, configPath string, externalNATSURL string, logger *slog.Logger) error {
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return fmt.Errorf("load hub config: %w", err)
	}

	conn, err := hub.StartNATS(ctx, cfg, externalNATSURL, logger)
	if err != nil {
		return fmt.Errorf("start nats: %w", err)
	}
	defer conn.Close()

	selfBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	sup := hub.NewSupervisor(cfg, conn.ConnectedUrl(), selfBin, logger)
	if err := sup.Start(ctx); err != nil {
		return fmt.Errorf("start supervisor: %w", err)
	}

	logger.Info("hub started",
		"name", cfg.Hub.Name,
		"agents", len(cfg.Agents),
	)

	// Block until context is cancelled (signal or parent shutdown).
	<-ctx.Done()
	logger.Info("hub shutting down")
	return nil
}

// HubStatus prints the current hub status.
// Phase 2 stub — will query the supervisor via Unix socket.
func HubStatus() error {
	fmt.Println("Hub status: not running")
	fmt.Println("(Phase 2 will query the supervisor process via Unix socket)")
	return nil
}
