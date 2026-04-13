package app

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/transport"
)

// AttachAgent connects a terminal session to a running agent daemon.
func AttachAgent(ctx context.Context, agentName, sessionID string, logger *slog.Logger) error {
	sockPath := filepath.Join(config.ZlawHome(), "run", agentName+".sock")
	t := transport.NewUnixTransport(sockPath)
	return cli.Attach(ctx, t, sessionID, logger)
}
