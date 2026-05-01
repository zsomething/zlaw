package executor

import (
	"context"
	"fmt"
	"io"
)

// SystemdExecutor manages agents via systemd services.
type SystemdExecutor struct{}

// Start creates and starts a systemd service for the agent.
func (e *SystemdExecutor) Start(ctx context.Context, cfg AgentConfig) error {
	return fmt.Errorf("systemd executor not yet implemented")
}

// Stop stops and optionally removes the systemd service.
func (e *SystemdExecutor) Stop(ctx context.Context, id string) error {
	return fmt.Errorf("systemd executor not yet implemented")
}

// Status returns the systemd service status.
func (e *SystemdExecutor) Status(ctx context.Context, id string) (Status, error) {
	return Status{}, fmt.Errorf("systemd executor not yet implemented")
}

// Logs returns logs from journalctl.
func (e *SystemdExecutor) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("systemd executor not yet implemented")
}
