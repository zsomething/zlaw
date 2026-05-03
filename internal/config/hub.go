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
	RemoveAgent(id string) error
	AddAgent(entry AgentEntry) error
	FindAgent(id string) (AgentEntry, bool)
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

// EnvVarMapping maps an env var name to a secret name.
type EnvVarMapping struct {
	// Name is the env var name injected to agent.
	Name string `toml:"name"`
	// FromSecret is the key in secrets.toml.
	FromSecret string `toml:"from_secret"`
}

// AgentEntry describes a single agent supervised by the hub.
// Hub knows only what it needs to manage the process lifecycle.
type AgentEntry struct {
	// ID is the stable machine-readable agent identifier.
	ID string `toml:"id"`
	// Dir is the absolute path to the agent's self-contained root (ZLAW_AGENT_HOME).
	// Contains agent.toml, SOUL.md, sessions/, memories/, workspace/.
	// When empty, defaults to $ZLAW_HOME/agents/<id>.
	Dir string `toml:"dir"`
	// Binary is the path to the agent executable.
	// When empty, defaults to the hub's own executable (zlaw agent serve).
	Binary string `toml:"binary"`
	// Executor defines how the agent is spawned and supervised.
	// Valid values: "subprocess", "systemd", "docker". Defaults to "subprocess".
	Executor string `toml:"executor"`
	// Target defines where the agent runs.
	// Valid values: "local", "ssh". Defaults to "local".
	Target string `toml:"target"`
	// TargetSSH is the SSH connection string for remote agents.
	// Required when target="ssh". Example: "user@host:2222".
	TargetSSH string `toml:"target_ssh"`
	// RestartPolicy controls when the supervisor restarts a crashed agent.
	// Valid values: "always", "on-failure", "never". Defaults to "on-failure".
	RestartPolicy RestartPolicy `toml:"restart_policy"`
	// Disabled prevents the hub from spawning or respawning this agent.
	Disabled bool `toml:"disabled,omitempty"`
	// EnvVars lists the env vars to inject from secrets.toml.
	EnvVars []EnvVarMapping `toml:"env_vars,omitempty"`
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

// RemoveAgent removes the agent entry with the given ID from zlaw.toml.
// It writes the updated config back to disk atomically.
func (c HubConfig) RemoveAgent(id string) error {
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
		if entry["id"] != id {
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

	newEntry := map[string]any{"id": entry.ID}
	if entry.Dir != "" {
		newEntry["dir"] = entry.Dir
	}
	if entry.Binary != "" {
		newEntry["binary"] = entry.Binary
	}
	if entry.RestartPolicy != "" {
		newEntry["restart_policy"] = string(entry.RestartPolicy)
	}
	if entry.Disabled {
		newEntry["disabled"] = true
	}
	if len(entry.EnvVars) > 0 {
		newEntry["env_vars"] = entry.EnvVars
	}
	agents = append(agents, newEntry)
	raw["agents"] = agents

	return writeHubConfig(path, raw)
}

// FindAgent returns the AgentEntry for id if present.
func (c HubConfig) FindAgent(id string) (AgentEntry, bool) {
	for _, a := range c.Agents {
		if a.ID == id {
			return a, true
		}
	}
	return AgentEntry{}, false
}

// Save writes the HubConfig back to the default path.
// It preserves any comments and ordering in the existing file where possible.
func (c HubConfig) Save() error {
	path := DefaultHubConfigPath()
	return c.SaveTo(path)
}

// SaveTo writes the HubConfig to the specified path.
func (c HubConfig) SaveTo(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Ensure agents section exists
	if _, ok := raw["agents"].([]map[string]any); !ok {
		raw["agents"] = []map[string]any{}
	}

	// Update agents section.
	agents := make([]map[string]any, len(c.Agents))
	for i, a := range c.Agents {
		entry := map[string]any{"id": a.ID}
		if a.Dir != "" {
			entry["dir"] = a.Dir
		}
		if a.Binary != "" {
			entry["binary"] = a.Binary
		}
		if a.RestartPolicy != "" {
			entry["restart_policy"] = string(a.RestartPolicy)
		}
		if a.Disabled {
			entry["disabled"] = true
		}
		if len(a.EnvVars) > 0 {
			entry["env_vars"] = a.EnvVars
		}
		agents[i] = entry
	}
	raw["agents"] = agents

	// Update hub section.
	if raw["hub"] == nil {
		raw["hub"] = map[string]any{}
	}
	if hub, ok := raw["hub"].(map[string]any); ok {
		if c.Hub.Name != "" {
			hub["name"] = c.Hub.Name
		}
		if c.Hub.Description != "" {
			hub["description"] = c.Hub.Description
		}
		if c.Hub.KeypairPath != "" {
			hub["keypair_path"] = c.Hub.KeypairPath
		}
		if c.Hub.AuditLogPath != "" {
			hub["audit_log_path"] = c.Hub.AuditLogPath
		}
	}

	return writeHubConfig(path, raw)
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

// SetAgentDisabled updates the disabled flag for agent ID in zlaw.toml.
func (c HubConfig) SetAgentDisabled(id string, disabled bool) error {
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
		return fmt.Errorf("agents section not found in %s", path)
	}

	found := false
	for _, entry := range agents {
		if entry["id"] == id {
			if disabled {
				entry["disabled"] = true
			} else {
				delete(entry, "disabled")
			}
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("agent %q not found in %s", id, path)
	}
	raw["agents"] = agents

	return writeHubConfig(path, raw)
}
