package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// HubConfig holds the top-level configuration for a zlaw-hub instance.
// It is loaded from $ZLAW_HOME/zlaw.toml.
type HubConfig struct {
	Hub    HubMeta      `toml:"hub"`
	Agents []AgentEntry `toml:"agents"`
	NATS   NATSConfig   `toml:"nats"`
}

// HubMeta holds hub identity metadata.
type HubMeta struct {
	Name         string `toml:"name"`
	Description  string `toml:"description"`
	KeypairPath  string `toml:"keypair_path"`
	AuditLogPath string `toml:"audit_log_path"`
}

// RestartPolicy controls when the supervisor restarts a crashed agent process.
type RestartPolicy string

const (
	// RestartAlways restarts the agent regardless of exit code.
	RestartAlways RestartPolicy = "always"
	// RestartOnFailure restarts the agent only when it exits with a non-zero code.
	RestartOnFailure RestartPolicy = "on-failure"
	// RestartNever never restarts the agent.
	RestartNever RestartPolicy = "never"
)

// AgentEntry describes a single agent supervised by the hub.
type AgentEntry struct {
	// Name is the logical agent name.
	Name string `toml:"name"`
	// Dir is the path to the agent's config directory (agent.toml, SOUL.md, etc.).
	// When empty, defaults to $ZLAW_HOME/agents/<name>.
	Dir string `toml:"dir"`
	// Binary is the path to the agent executable.
	// When empty, defaults to the hub's own executable (zlaw agent serve).
	Binary string `toml:"binary"`
	// RestartPolicy controls when the supervisor restarts a crashed agent.
	// Valid values: "always", "on-failure", "never". Defaults to "on-failure".
	RestartPolicy RestartPolicy `toml:"restart_policy"`
	// Manager marks this agent as the manager agent.
	// Manager agents receive broader NATS publish permissions (agent.*.inbox,
	// zlaw.hub.inbox) and can subscribe to zlaw.registry.
	// Only one agent per hub should be marked as manager.
	Manager bool `toml:"manager"`
}

// NATSConfig holds embedded NATS server settings.
type NATSConfig struct {
	// Listen is the address for the embedded NATS server.
	// Defaults to "127.0.0.1:4222" when empty.
	Listen string `toml:"listen"`
}

// DefaultHubConfigPath returns the path to zlaw.toml.
func DefaultHubConfigPath() string {
	return filepath.Join(ZlawHome(), "zlaw.toml")
}

// LoadHubConfig reads and parses zlaw.toml from path.
func LoadHubConfig(path string) (HubConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HubConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	expanded := os.Expand(string(data), os.Getenv)
	var cfg HubConfig
	if _, err := toml.Decode(expanded, &cfg); err != nil {
		return HubConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}
