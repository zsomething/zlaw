// Package auth provides pluggable authentication for LLM backends.
//
// Credentials are stored in ~/.config/zlaw/credentials.toml (path overridable
// via ZLAW_CREDENTIALS_FILE). The agent.toml references a named profile via
// llm.auth_profile — no secrets ever appear in agent config.
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/zsomething/zlaw/internal/config"
)

// TokenSource returns a bearer token for use in an Authorization header.
// Implementations must be safe for concurrent use.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// ProfileType identifies the authentication mechanism.
type ProfileType string

const (
	ProfileTypeAPIKey ProfileType = "apikey"
	ProfileTypeOAuth2 ProfileType = "oauth2"
)

// CredentialProfile is one entry in credentials.toml.
type CredentialProfile struct {
	Type ProfileType `toml:"type"`

	// apikey fields
	Key string `toml:"key"`

	// oauth2 fields
	AccessToken  string    `toml:"access_token"`
	TokenType    string    `toml:"token_type"`
	RefreshToken string    `toml:"refresh_token"`
	Expiry       time.Time `toml:"expiry"`

	// oauth2 client credentials (used for refresh; written by auth login)
	TokenURL     string `toml:"token_url"`
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
	Scope        string `toml:"scope"`
}

// CredentialStore is the in-memory representation of credentials.toml.
type CredentialStore struct {
	Profiles map[string]CredentialProfile `toml:"profiles"`
}

// ErrProfileNotFound is returned when a named profile does not exist in the store.
var ErrProfileNotFound = errors.New("auth: credential profile not found")

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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
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

// NewTokenSource constructs the appropriate TokenSource for profile.
func NewTokenSource(profile CredentialProfile) (TokenSource, error) {
	switch profile.Type {
	case ProfileTypeAPIKey:
		if profile.Key == "" {
			return nil, fmt.Errorf("auth: apikey profile has empty key")
		}
		return &staticKeySource{key: profile.Key}, nil
	case ProfileTypeOAuth2:
		return &oauth2Source{profile: profile, mu: &sync.Mutex{}}, nil
	default:
		return nil, fmt.Errorf("auth: unknown profile type %q", profile.Type)
	}
}

// NewTokenSourceFromStore loads the named profile and returns a TokenSource.
func NewTokenSourceFromStore(path, profileName string) (TokenSource, error) {
	store, err := LoadStore(path)
	if err != nil {
		return nil, err
	}
	profile, ok := store.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrProfileNotFound, profileName)
	}
	return NewTokenSource(profile)
}
