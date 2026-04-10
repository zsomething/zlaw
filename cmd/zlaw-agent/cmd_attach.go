package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/transport"
)

func runAttach(ctx context.Context, args []string, agentName, agentDir string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	sessionID := fs.String("session", "default", "session ID to attach to")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Resolve the agent name for the socket path.
	name := agentName
	if name == "" {
		// Fall back to the base name of agentDir.
		if agentDir == "" {
			return fmt.Errorf("attach: --agent <name> is required (or set ZLAW_AGENT)")
		}
		name = filepath.Base(agentDir)
	}

	sockPath := filepath.Join(config.ZlawHome(), "run", name+".sock")
	t := transport.NewUnixTransport(sockPath)

	return cli.Attach(ctx, t, *sessionID, logger)
}
