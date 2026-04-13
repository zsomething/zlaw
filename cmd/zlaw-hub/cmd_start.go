package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/chickenzord/zlaw/internal/config"
)

// runStart loads zlaw.toml, prints a config summary, then stubs hub startup.
// Full implementation is wired in Phase 2 once the supervisor exists.
func runStart(ctx context.Context, args []string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	configPath := fs.String("config", config.DefaultHubConfigPath(), "path to zlaw.toml")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub start [--config <path>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadHubConfig(*configPath)
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
	return nil
}
