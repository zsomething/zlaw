package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// WriteLLMConfig writes LLM configuration to agent.toml.
func WriteLLMConfig(agentDir string, llm LLMConfig) error {
	path := filepath.Join(agentDir, "agent.toml")

	// Read existing config if present.
	var existing map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &existing); err != nil {
			// Decode error, use empty config.
			existing = make(map[string]any)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read agent.toml: %w", err)
	}

	if existing == nil {
		existing = make(map[string]any)
	}

	// Encode LLM config.
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(map[string]any{"llm": llm}); err != nil {
		return fmt.Errorf("encode llm config: %w", err)
	}

	// Parse and merge.
	var newLLM map[string]any
	if _, err := toml.Decode(buf.String(), &newLLM); err != nil {
		return fmt.Errorf("parse llm config: %w", err)
	}

	existing["llm"] = newLLM["llm"]

	// Write back.
	return writeAgentConfig(path, existing)
}

// writeAgentConfig writes the agent config to path.
func writeAgentConfig(path string, cfg map[string]any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return fmt.Errorf("encode agent config: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// WriteAdapterConfig writes adapter configuration to agent.toml.
// Pass nil to clear the adapter config.
func WriteAdapterConfig(agentDir string, adapter *AdapterInstanceConfig) error {
	path := filepath.Join(agentDir, "agent.toml")

	// Read existing config if present.
	var existing map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(data), &existing); err != nil {
			existing = make(map[string]any)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read agent.toml: %w", err)
	}

	if existing == nil {
		existing = make(map[string]any)
	}

	if adapter == nil {
		// Clear adapter config.
		delete(existing, "adapter")
	} else {
		// Encode adapter config.
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(map[string]any{"adapter": []AdapterInstanceConfig{*adapter}}); err != nil {
			return fmt.Errorf("encode adapter config: %w", err)
		}

		var newAdapter map[string]any
		if _, err := toml.Decode(buf.String(), &newAdapter); err != nil {
			return fmt.Errorf("parse adapter config: %w", err)
		}

		existing["adapter"] = newAdapter["adapter"]
	}

	return writeAgentConfig(path, existing)
}

// SetAgentEnvVar adds or updates an env var mapping for an agent in zlaw.toml.
func SetAgentEnvVar(agentID, envName, secretName string) error {
	path := DefaultHubConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read zlaw.toml: %w", err)
	}

	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return fmt.Errorf("parse zlaw.toml: %w", err)
	}

	agents, ok := raw["agents"].([]map[string]any)
	if !ok {
		return fmt.Errorf("agents section not found")
	}

	for _, entry := range agents {
		if entry["id"] == agentID {
			envVars, ok := entry["env_vars"].([]map[string]any)
			if !ok {
				envVars = []map[string]any{}
				entry["env_vars"] = envVars
			}

			// Check if env var already exists, update or add.
			found := false
			for i, ev := range envVars {
				if ev["name"] == envName {
					envVars[i]["from_secret"] = secretName
					found = true
					break
				}
			}
			if !found {
				envVars = append(envVars, map[string]any{
					"name":        envName,
					"from_secret": secretName,
				})
				entry["env_vars"] = envVars
			}

			return writeHubConfig(path, raw)
		}
	}

	return fmt.Errorf("agent %q not found", agentID)
}

// AddSecret adds a secret to secrets.toml.
func AddSecret(name, value string) error {
	secPath := filepath.Join(ZlawHome(), "secrets.toml")

	// Load existing secrets.
	store, err := LoadSecrets(secPath)
	if err != nil {
		store = make(map[string]string)
	}

	store[name] = value

	return SaveSecrets(secPath, store)
}

// LoadSecrets loads secrets.toml.
func LoadSecrets(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}

	var store map[string]string
	if _, err := toml.Decode(string(data), &store); err != nil {
		return nil, err
	}
	return store, nil
}

// SaveSecrets writes secrets store to path.
func SaveSecrets(path string, store map[string]string) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(store); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// ListSecrets returns secret names.
func ListSecrets() []string {
	path := filepath.Join(ZlawHome(), "secrets.toml")
	store, err := LoadSecrets(path)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(store))
	for k := range store {
		names = append(names, k)
	}
	return names
}

// RemoveSecret deletes a secret by name.
func RemoveSecret(name string) error {
	secPath := filepath.Join(ZlawHome(), "secrets.toml")

	store, err := LoadSecrets(secPath)
	if err != nil {
		return err
	}

	if _, exists := store[name]; !exists {
		return fmt.Errorf("secret %q not found", name)
	}

	delete(store, name)
	return SaveSecrets(secPath, store)
}
