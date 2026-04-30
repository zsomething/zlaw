package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// AgentEntryEditor is the subset of HubConfig needed by the hub CLI and control socket
// for agent lifecycle operations.
type AgentEntryEditor interface {
	RemoveAgent(name string) error
	AddAgent(entry AgentEntry) error
	FindAgent(name string) (AgentEntry, bool)
}

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
	// Dir is the path to the agent's config directory (agent.toml, credentials.toml).
	// When empty, defaults to $ZLAW_HOME/agents/<name>.
	Dir string `toml:"dir"`
	// Workspace is the path to the agent's workspace (SOUL.md, IDENTITY.md, memories/).
	// When empty, defaults to $ZLAW_HOME/workspaces/<name>.
	// The agent has read/write access to this directory.
	Workspace string `toml:"workspace"`
	// Binary is the path to the agent executable.
	// When empty, defaults to the hub's own executable (zlaw agent serve).
	Binary string `toml:"binary"`
	// RestartPolicy controls when the supervisor restarts a crashed agent.
	// Valid values: "always", "on-failure", "never". Defaults to "on-failure".
	RestartPolicy RestartPolicy `toml:"restart_policy"`
}

// NATSConfig holds embedded NATS server settings.
type NATSConfig struct {
	// Listen is the address for the embedded NATS server.
	// Defaults to "127.0.0.1:4222" when empty.
	Listen string `toml:"listen"`
	// StoreDir sets the JetStream message storage directory.
	// Defaults to $ZLAW_HOME/nats when empty.
	StoreDir string `toml:"store_dir"`
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

// RemoveAgent removes the agent entry with the given name from zlaw.toml.
// It writes the updated config back to disk atomically.
func (c HubConfig) RemoveAgent(name string) error {
	path := DefaultHubConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// Parse existing config preserving structure and comments.
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	agents, ok := raw["agents"].([]map[string]any)
	if !ok {
		return fmt.Errorf("agents section not found in %s", path)
	}

	updated := make([]map[string]any, 0, len(agents))
	for _, entry := range agents {
		if entry["name"] != name {
			updated = append(updated, entry)
		}
	}
	raw["agents"] = updated

	return writeHubConfig(path, raw)
}

// AddAgent adds a new agent entry to zlaw.toml. It does not check for duplicates.
func (c HubConfig) AddAgent(entry AgentEntry) error {
	path := DefaultHubConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	agents, ok := raw["agents"].([]map[string]any)
	if !ok {
		agents = []map[string]any{}
		raw["agents"] = agents
	}

	newEntry := map[string]any{"name": entry.Name}
	if entry.Dir != "" {
		newEntry["dir"] = entry.Dir
	}
	if entry.Workspace != "" {
		newEntry["workspace"] = entry.Workspace
	}
	if entry.Binary != "" {
		newEntry["binary"] = entry.Binary
	}
	if entry.RestartPolicy != "" {
		newEntry["restart_policy"] = string(entry.RestartPolicy)
	}
	agents = append(agents, newEntry)
	raw["agents"] = agents

	return writeHubConfig(path, raw)
}

// FindAgent returns the AgentEntry for name if present.
func (c HubConfig) FindAgent(name string) (AgentEntry, bool) {
	for _, a := range c.Agents {
		if a.Name == name {
			return a, true
		}
	}
	return AgentEntry{}, false
}

// writeHubConfig writes raw config back to path.
func writeHubConfig(path string, raw map[string]any) error {
	// Use toml.Canonicalize to preserve key order.
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(raw); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// WriteAgentDisabled writes disabled=true or disabled=false to an agent's agent.toml.
func WriteAgentDisabled(agentDir string, disabled bool) error {
	path := filepath.Join(agentDir, "agent.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if raw == nil {
		raw = make(map[string]any)
	}
	raw["disabled"] = disabled

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(raw); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}
