package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/zsomething/zlaw/internal/app"
	"github.com/zsomething/zlaw/internal/logging"
)

type AgentServeCmd struct {
	AgentFlags
}

func (c *AgentServeCmd) Run(ctx context.Context, logger *slog.Logger) error {
	agentDir, err := c.resolveDir()
	if err != nil {
		return err
	}

	// Wrap logger with agent label for PrettyHandler mode.
	// When ZLAW_LOG_FORMAT=json (hub-spawned), the agent outputs JSON and
	// hub captures/reformats it. In standalone mode, use PrettyHandler.
	name := os.Getenv("ZLAW_AGENT")
	if name == "" {
		name = c.Agent
	}
	if name != "" && logging.DetectFormat() != logging.LogFormatJSON {
		label := "[agent:" + name + "]"
		color := logging.AgentColor(name)
		opts := logging.Options{
			Label:   label,
			Color:   color,
			NoColor: logging.DetectNoColor(),
			Time:    logging.DetectTimeFormat(),
		}
		logger = logging.LoggerWithOptions(opts)
	}

	return app.ServeAgent(ctx, agentDir, logger)
}
