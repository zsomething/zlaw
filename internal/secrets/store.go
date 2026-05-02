// Package secrets provides secret storage and management.
//
// Secrets are stored in $ZLAW_HOME/secrets.toml as flat key-value pairs.
// ctl owns and manages secrets; agents receive values via env vars at spawn.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/zsomething/zlaw/internal/config"
)

// Store is the in-memory representation of secrets.toml.
// Keys are secret names, values are secret values.
type Store map[string]string

// DefaultSecretsPath returns the path to secrets.toml.
func DefaultSecretsPath() string {
	return filepath.Join(config.ZlawHome(), "secrets.toml")
}

// Load reads and parses secrets.toml from path.
// Returns an empty store (no error) if the file does not exist.
func Load(path string) (Store, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Store{}, nil
	}
	if err != nil {
		return Store{}, fmt.Errorf("read secrets: %w", err)
	}

	var store Store
	expanded := os.Expand(string(data), os.Getenv)
	if _, err := toml.Decode(expanded, &store); err != nil {
		return Store{}, fmt.Errorf("parse secrets: %w", err)
	}
	return store, nil
}

// Save writes store to path with 0600 permissions, creating parent dirs as needed.
func Save(path string, store Store) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open secrets file: %w", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(store)
}

// Get returns the value for key. Returns empty string if key not found.
func (s Store) Get(key string) string {
	return s[key]
}

// Set sets a key-value pair. Call Save to persist.
func (s Store) Set(key, value string) {
	s[key] = value
}

// Delete removes a key. Call Save to persist.
func (s Store) Delete(key string) {
	delete(s, key)
}

// List returns all secret names (keys). Values are not included.
func (s Store) List() []string {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	return keys
}
