package executor

import (
	"context"
	"fmt"
	"io"
)

// DockerExecutor manages agents via Docker containers.
type DockerExecutor struct{}

// Start creates and starts a Docker container for the agent.
func (e *DockerExecutor) Start(ctx context.Context, cfg AgentConfig) error {
	return fmt.Errorf("docker executor not yet implemented")
}

// Stop stops and removes the Docker container.
func (e *DockerExecutor) Stop(ctx context.Context, id string) error {
	return fmt.Errorf("docker executor not yet implemented")
}

// Status returns the Docker container status.
func (e *DockerExecutor) Status(ctx context.Context, id string) (Status, error) {
	return Status{}, fmt.Errorf("docker executor not yet implemented")
}

// Logs returns logs from docker logs.
func (e *DockerExecutor) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("docker executor not yet implemented")
}
