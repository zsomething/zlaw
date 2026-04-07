// Package config handles loading and hot-reloading of per-agent configuration.
package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// AgentConfig holds all configuration for a single agent instance.
type AgentConfig struct {
	Agent   AgentMeta   `toml:"agent"`
	LLM     LLMConfig   `toml:"llm"`
	Tools   ToolsConfig `toml:"tools"`
	Adapter AdapterConfig `toml:"adapter"`
}

// AgentMeta contains agent identity metadata.
type AgentMeta struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// LLMConfig holds LLM backend settings.
// Secrets (e.g. API keys) are loaded from environment variables named by the
// *Env fields — never stored as plaintext in the config file.
type LLMConfig struct {
	Backend    string `toml:"backend"`     // e.g. "anthropic"
	Model      string `toml:"model"`       // e.g. "claude-opus-4-6"
	APIKeyEnv  string `toml:"api_key_env"` // env var name holding the API key
	MaxTokens  int    `toml:"max_tokens"`
	TimeoutSec int    `toml:"timeout_sec"`
}

// APIKey resolves the LLM API key from the environment.
func (l LLMConfig) APIKey() (string, error) {
	if l.APIKeyEnv == "" {
		return "", fmt.Errorf("llm.api_key_env is not set")
	}
	val := os.Getenv(l.APIKeyEnv)
	if val == "" {
		return "", fmt.Errorf("env var %s is empty or unset", l.APIKeyEnv)
	}
	return val, nil
}

// ToolsConfig lists the tools this agent is allowed to invoke.
type ToolsConfig struct {
	Allowed []string `toml:"allowed"`
}

// AdapterConfig selects and configures the I/O adapter.
type AdapterConfig struct {
	Type string `toml:"type"` // "cli", "telegram", etc.
}

// Personality holds the raw contents of SOUL.md and IDENTITY.md.
type Personality struct {
	Soul     string
	Identity string
}

// Loader loads and watches configuration files for a single agent.
type Loader struct {
	dir      string
	onChange func(AgentConfig, Personality)
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
}

// NewLoader creates a Loader for the given agent directory.
// onChange is called whenever any watched file changes; it receives the newly
// loaded config and personality. The callback must not mutate shared state
// directly — the caller is responsible for safe state updates.
func NewLoader(dir string, onChange func(AgentConfig, Personality), logger *slog.Logger) (*Loader, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	return &Loader{dir: dir, onChange: onChange, watcher: w, logger: logger}, nil
}

// Load reads agent.toml, SOUL.md, and IDENTITY.md from the agent directory
// and returns the parsed values.
func (l *Loader) Load() (AgentConfig, Personality, error) {
	cfg, err := loadTOML(l.dir + "/agent.toml")
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}
	p, err := loadPersonality(l.dir)
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}
	return cfg, p, nil
}

// Watch starts watching the agent directory for changes. It blocks until ctx
// is cancelled. Call in a goroutine.
func (l *Loader) Watch(ctx context.Context) error {
	if err := l.watcher.Add(l.dir); err != nil {
		return fmt.Errorf("watch dir %s: %w", l.dir, err)
	}
	defer l.watcher.Close()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-l.watcher.Events:
			if !ok {
				return nil
			}
			if !isRelevantEvent(event) {
				continue
			}
			l.logger.Info("config file changed, reloading", "file", event.Name)
			cfg, p, err := l.Load()
			if err != nil {
				l.logger.Error("reload failed", "err", err)
				continue
			}
			l.onChange(cfg, p)
		case err, ok := <-l.watcher.Errors:
			if !ok {
				return nil
			}
			l.logger.Error("watcher error", "err", err)
		}
	}
}

// loadTOML parses agent.toml, expanding ${ENV_VAR} references in string values.
func loadTOML(path string) (AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	expanded := os.Expand(string(data), func(key string) string {
		return os.Getenv(key)
	})
	var cfg AgentConfig
	if _, err := toml.Decode(expanded, &cfg); err != nil {
		return AgentConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// loadPersonality reads SOUL.md and IDENTITY.md; missing files are silently
// treated as empty strings.
func loadPersonality(dir string) (Personality, error) {
	soul, err := readOptional(dir + "/SOUL.md")
	if err != nil {
		return Personality{}, err
	}
	identity, err := readOptional(dir + "/IDENTITY.md")
	if err != nil {
		return Personality{}, err
	}
	return Personality{Soul: soul, Identity: identity}, nil
}

func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func isRelevantEvent(e fsnotify.Event) bool {
	if e.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}
	name := e.Name
	return strings.HasSuffix(name, "agent.toml") ||
		strings.HasSuffix(name, "SOUL.md") ||
		strings.HasSuffix(name, "IDENTITY.md")
}
