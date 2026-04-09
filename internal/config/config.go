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
// Authentication is handled separately via a named credential profile in
// ~/.config/zlaw/credentials.toml — no secrets belong in agent.toml.
type LLMConfig struct {
	// Backend selects a named preset (e.g. "minimax", "minimax-cn", "openrouter").
	Backend string `toml:"backend"`

	// APIFormat overrides the wire protocol from the preset ("openai" or "anthropic").
	// Leave empty to use the preset default.
	APIFormat string `toml:"api_format"`

	// BaseURL overrides the endpoint URL from the preset.
	// Leave empty to use the preset default.
	BaseURL string `toml:"base_url"`

	Model       string `toml:"model"`        // e.g. "MiniMax-Text-01"
	AuthProfile string `toml:"auth_profile"` // profile name in credentials.toml
	MaxTokens   int    `toml:"max_tokens"`
	TimeoutSec  int    `toml:"timeout_sec"`

	// ContextTokenBudget is the maximum estimated token count of the message
	// history sent to the LLM. Oldest conversation turns are pruned when the
	// estimate exceeds this value. Zero disables pruning.
	ContextTokenBudget int `toml:"context_token_budget"`

	// ContextSummarizeThreshold is the fraction of ContextTokenBudget at which
	// summarization is triggered before falling back to pruning (e.g. 0.8 = 80%).
	// Zero disables summarization; summarization only applies when ContextTokenBudget > 0.
	ContextSummarizeThreshold float64 `toml:"context_summarize_threshold"`

	// ContextSummarizeTurns is how many of the oldest turns to collapse into a
	// single summary message per summarization pass. Zero uses a default of 10.
	ContextSummarizeTurns int `toml:"context_summarize_turns"`

	// ContextPruneLevels is an ordered list of pruning strategies applied after
	// summarization. Supported values: "strip_thinking", "strip_tool_results",
	// "drop_pairs". Empty defaults to ["drop_pairs"] for backward compatibility.
	ContextPruneLevels []string `toml:"context_prune_levels"`
}

// ToolsConfig lists the tools this agent is allowed to invoke.
type ToolsConfig struct {
	Allowed        []string `toml:"allowed"`
	MaxResultBytes int      `toml:"max_result_bytes"`
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
			if l.onChange != nil {
				l.onChange(cfg, p)
			}
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
