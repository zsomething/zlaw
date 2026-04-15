package app

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/adapters/cli"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/transport"
)

// AttachAgent connects a terminal session to a running agent daemon.
func AttachAgent(ctx context.Context, agentName, sessionID string, logger *slog.Logger) error {
	sockPath := filepath.Join(config.ZlawHome(), "run", "agent-"+agentName+".sock")
	t := transport.NewUnixTransport(sockPath)
	return cli.Attach(ctx, t, sessionID, logger)
}
