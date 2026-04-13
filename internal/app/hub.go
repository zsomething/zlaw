package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsomething/zlaw/internal/config"
)

// StartHub loads the hub config and starts the hub process.
// Phase 2 stub — full supervisor not yet implemented.
func StartHub(ctx context.Context, configPath string, logger *slog.Logger) error {
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return fmt.Errorf("load hub config: %w", err)
	}

	fmt.Println("Hub configuration:")
	fmt.Printf("  name:        %s\n", cfg.Hub.Name)
	fmt.Printf("  description: %s\n", cfg.Hub.Description)
	if len(cfg.Agents.Names) > 0 {
		fmt.Printf("  agents:      %v\n", cfg.Agents.Names)
	} else {
		fmt.Println("  agents:      (none configured)")
	}
	if cfg.NATS.Listen != "" {
		fmt.Printf("  nats listen: %s\n", cfg.NATS.Listen)
	} else {
		fmt.Println("  nats listen: 127.0.0.1:4222 (default)")
	}
	fmt.Println()
	fmt.Println("Starting hub... (stub — supervisor not yet implemented)")
	fmt.Println("Phase 2 will wire: embedded NATS, agent supervisor, registry, credential injection.")

	_ = logger
	_ = ctx
	return nil
}

// HubStatus prints the current hub status.
// Phase 2 stub — will query the supervisor via Unix socket.
func HubStatus() error {
	fmt.Println("Hub status: not running")
	fmt.Println("(Phase 2 will query the supervisor process via Unix socket)")
	return nil
}
