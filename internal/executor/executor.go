// Package executor provides execution abstraction for agent processes.
// It supports multiple executors (subprocess, systemd, docker) and targets (local, ssh).
package executor

import (
	"context"
	"io"
)

// Status represents the current state of an agent.
type Status struct {
	ID      string
	Running bool
	PID     int
	Error   error
}

// AgentConfig holds configuration for an agent executor.
type AgentConfig struct {
	ID            string
	Dir           string
	Binary        string
	Executor      string   // "subprocess", "systemd", "docker"
	Target        string   // "local", "ssh"
	TargetSSH     string   // SSH connection string
	RestartPolicy string   // "always", "on-failure", "never"
	NATSURL       string   // NATS server URL (e.g., nats://127.0.0.1:4222)
	NATSToken     string   // NATS credentials token (optional)
	AuthProfiles  []string // Credential profiles to inject
}

// Executor is the interface for spawning and managing agent processes.
type Executor interface {
	// Start launches the agent.
	Start(ctx context.Context, cfg AgentConfig) error

	// Stop terminates the agent.
	Stop(ctx context.Context, id string) error

	// Status returns the current state of the agent.
	Status(ctx context.Context, id string) (Status, error)

	// Logs returns a stream of logs from the agent.
	Logs(ctx context.Context, id string) (io.ReadCloser, error)
}

// NewExecutor creates an executor for the given type.
func NewExecutor(executorType string) Executor {
	switch executorType {
	case "systemd":
		return &SystemdExecutor{}
	case "docker":
		return &DockerExecutor{}
	case "subprocess", "":
		return &SubprocessExecutor{}
	default:
		return &SubprocessExecutor{}
	}
}
