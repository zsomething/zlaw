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
	Hub    HubMeta    `toml:"hub"`
	Agents HubAgents  `toml:"agents"`
	NATS   NATSConfig `toml:"nats"`
}

// HubMeta holds hub identity metadata.
type HubMeta struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// HubAgents lists the agents supervised by this hub.
type HubAgents struct {
	// Names is the list of agent names to supervise.
	// Each name maps to $ZLAW_HOME/agents/<name>/.
	Names []string `toml:"names"`
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
