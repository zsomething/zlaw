// Package credentials provides pluggable authentication for LLM backends and
// adapters.
//
// Credentials are stored in ~/.config/zlaw/credentials.toml (path overridable
// via ZLAW_CREDENTIALS_FILE). The agent.toml references a named profile via
// llm.auth_profile or adapter.auth_profile — no secrets ever appear in agent config.
package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/zsomething/zlaw/internal/config"
)

// TokenSource returns a bearer token for use in an Authorization header.
// Implementations must be safe for concurrent use.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// CredentialProfile is one entry in credentials.toml.
// Each adapter decides which keys from Data it needs (e.g. "telegram_bot_token",
// "fizzy_api_key"). This keeps the struct generic and extensible.
type CredentialProfile struct {
	Name string            `toml:"name"`
	Data map[string]string `toml:"data"`
}

// CredentialStore is the in-memory representation of credentials.toml.
type CredentialStore struct {
	Profiles map[string]CredentialProfile `toml:"profiles"`
}

// ErrProfileNotFound is returned when a named profile does not exist in the store.
var ErrProfileNotFound = errors.New("credentials: profile not found")

// DefaultCredentialsPath returns the path to credentials.toml.
// Priority: ZLAW_CREDENTIALS_FILE env var → $ZLAW_HOME/credentials.toml.
func DefaultCredentialsPath() string {
	if v := os.Getenv("ZLAW_CREDENTIALS_FILE"); v != "" {
		return v
	}
	return filepath.Join(config.ZlawHome(), "credentials.toml")
}

// LoadStore reads and parses credentials.toml from path.
// Returns an empty store (no error) if the file does not exist.
func LoadStore(path string) (CredentialStore, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return CredentialStore{Profiles: map[string]CredentialProfile{}}, nil
	}
	if err != nil {
		return CredentialStore{}, fmt.Errorf("read credentials: %w", err)
	}
	var store CredentialStore
	expanded := os.Expand(string(data), os.Getenv)
	if _, err := toml.Decode(expanded, &store); err != nil {
		return CredentialStore{}, fmt.Errorf("parse credentials: %w", err)
	}
	if store.Profiles == nil {
		store.Profiles = map[string]CredentialProfile{}
	}
	return store, nil
}

// SaveStore writes store to path with 0600 permissions, creating parent dirs
// as needed. It warns (to stderr) if existing file has looser permissions.
func SaveStore(path string, store CredentialStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}

	// Warn if existing file is world/group readable.
	if info, err := os.Stat(path); err == nil {
		if info.Mode().Perm()&0077 != 0 {
			fmt.Fprintf(os.Stderr, "warning: credentials file %s has loose permissions (%s), tightening to 0600\n", path, info.Mode().Perm())
			_ = os.Chmod(path, 0600)
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open credentials file: %w", err)
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	return enc.Encode(store)
}

// UpsertProfile loads the store at path, sets the named profile, and saves.
func UpsertProfile(path, name string, profile CredentialProfile) error {
	store, err := LoadStore(path)
	if err != nil {
		return err
	}
	store.Profiles[name] = profile
	return SaveStore(path, store)
}

// GetProfile loads the named profile from the store.
func GetProfile(path, name string) (CredentialProfile, error) {
	store, err := LoadStore(path)
	if err != nil {
		return CredentialProfile{}, err
	}
	profile, ok := store.Profiles[name]
	if !ok {
		return CredentialProfile{}, fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}
	return profile, nil
}

// NewTokenSourceFromStore loads the named profile and returns a TokenSource
// that extracts "api_key" from the profile's Data.
func NewTokenSourceFromStore(path, profileName string) (TokenSource, error) {
	profile, err := GetProfile(path, profileName)
	if err != nil {
		return nil, err
	}
	key, ok := profile.Data["api_key"]
	if !ok || key == "" {
		return nil, fmt.Errorf("credentials: profile %q has no api_key in data", profileName)
	}
	return &staticKeySource{key: key}, nil
}

// GetData extracts a value from the profile's Data map.
// Returns empty string if the key is not found.
func (p CredentialProfile) GetData(key string) string {
	return p.Data[key]
}

// staticKeySource returns a fixed API key.
type staticKeySource struct {
	key string
}

func (s *staticKeySource) Token(_ context.Context) (string, error) {
	return s.key, nil
}
