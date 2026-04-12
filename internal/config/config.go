// Package config handles loading and hot-reloading of per-agent configuration.
package config

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

// AgentConfig holds all configuration for a single agent instance.
type AgentConfig struct {
	Agent   AgentMeta     `toml:"agent"`
	LLM     LLMConfig     `toml:"llm"`
	Context ContextConfig `toml:"context"`
	Tools   ToolsConfig   `toml:"tools"`
	Adapter AdapterConfig `toml:"adapter"`
	Sticky  StickyConfig  `toml:"sticky"`
	Serve   ServeConfig   `toml:"serve"`
}

// ServeConfig controls daemon-mode behaviour.
type ServeConfig struct {
	// ShutdownTimeoutSec is how long (in seconds) the daemon waits for
	// in-flight agent turns to complete after receiving SIGTERM before
	// force-cancelling them. Defaults to 60.
	ShutdownTimeoutSec int `toml:"shutdown_timeout"`
}

// StickyConfig controls which built-in sticky context blocks are injected at
// the head of every system prompt. Sticky content is defined in Go source;
// these flags enable individual blocks.
type StickyConfig struct {
	// ProactiveMemorySave injects instructions for proactive long-term memory
	// saving. Implemented by card #229.
	ProactiveMemorySave bool `toml:"proactive_memory_save"`
}

// ContextConfig controls what contextual information is injected into the
// first user message of a new session.
type ContextConfig struct {
	// Prefill is an ordered list of context sources to inject at session start.
	// Supported sources: "cwd", "datetime", "file:<relative-path>".
	// Empty disables prefill injection (default).
	Prefill []string `toml:"prefill"`
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

	// ContextSummarizeModel is the model to use for summarization. When set,
	// summarization uses this model (same backend and auth profile) instead of
	// the main model. Useful for routing summarization to a cheaper/faster model.
	// Defaults to Model if empty.
	ContextSummarizeModel string `toml:"context_summarize_model"`

	// PromptCaching controls whether the system prompt is sent with
	// cache_control on the Anthropic backend. Nil or true = enabled (default);
	// explicitly false = disabled. Other backends ignore this field.
	PromptCaching *bool `toml:"prompt_caching"`

	// MaxMemoryTokens is the maximum number of tokens to include in the
	// injected [Memories] block appended to the system prompt. Memories are
	// ordered by recency (most recent first) and truncated to fit the budget.
	// Zero means no limit.
	MaxMemoryTokens int `toml:"max_memory_tokens"`
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

// RuntimeConfig holds mutable configuration that can be changed at runtime
// without restarting the process. Its values override the corresponding fields
// in AgentConfig after being merged in. Missing fields are ignored (zero value
// = no override).
type RuntimeConfig struct {
	LLM RuntimeLLMConfig `toml:"llm"`
}

// RuntimeLLMConfig holds the runtime-overridable LLM fields.
type RuntimeLLMConfig struct {
	// Model overrides LLMConfig.Model when non-empty.
	Model string `toml:"model"`
}

// runtimeFieldAllowlist is the canonical set of keys accepted by
// WriteRuntimeField. The configure tool schema mirrors this as an enum for
// first-layer validation; WriteRuntimeField re-validates as defense-in-depth.
var runtimeFieldAllowlist = map[string]struct{}{
	"llm.model": {},
}

// Loader loads and watches configuration files for a single agent.
type Loader struct {
	dir            string
	onChange       func(AgentConfig, Personality)
	onCronChange   func(CronConfig)
	watcher        *fsnotify.Watcher
	logger         *slog.Logger

	mu          sync.Mutex
	staticCfg   AgentConfig // from agent.toml; set once in Load, never mutated
	personality Personality // from SOUL.md / IDENTITY.md; updated on each Load
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

// Load reads agent.toml, runtime.toml, SOUL.md, and IDENTITY.md from the
// agent directory, merges the runtime overrides, and returns the result.
// It also caches the static config and personality for use by ReloadRuntime.
func (l *Loader) Load() (AgentConfig, Personality, error) {
	static, err := loadTOML(l.dir + "/agent.toml")
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}
	p, err := loadPersonality(l.dir)
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}
	rt, err := loadRuntimeTOML(l.dir + "/runtime.toml")
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}
	merged := mergeRuntime(static, rt)

	l.mu.Lock()
	l.staticCfg = static
	l.personality = p
	l.mu.Unlock()

	return merged, p, nil
}

// ReloadRuntime re-reads runtime.toml, merges it over the cached static
// config, and calls the onChange callback with the new effective config.
// It is safe to call concurrently from the fsnotify watcher goroutine and
// from tool executors (e.g. the configure tool).
func (l *Loader) ReloadRuntime() error {
	l.mu.Lock()
	static := l.staticCfg
	p := l.personality
	l.mu.Unlock()

	rt, err := loadRuntimeTOML(l.dir + "/runtime.toml")
	if err != nil {
		return fmt.Errorf("reload runtime config: %w", err)
	}
	merged := mergeRuntime(static, rt)

	if l.onChange != nil {
		l.onChange(merged, p)
	}
	return nil
}

// WriteRuntimeField validates key against the allowlist, then atomically
// writes the updated value to runtime.toml. It does not trigger a reload —
// the caller is responsible for calling ReloadRuntime() afterward.
func (l *Loader) WriteRuntimeField(key, value string) error {
	if _, ok := runtimeFieldAllowlist[key]; !ok {
		return fmt.Errorf("config: field %q is not runtime-configurable", key)
	}

	// Read existing runtime.toml (missing file → empty config).
	existing, err := loadRuntimeTOML(l.dir + "/runtime.toml")
	if err != nil {
		return fmt.Errorf("config: read runtime.toml before write: %w", err)
	}

	rt := existing
	switch key {
	case "llm.model":
		rt.LLM.Model = value
	default:
		// Defense-in-depth: allowlist check above should have caught this.
		return fmt.Errorf("config: field %q is not runtime-configurable", key)
	}

	return writeRuntimeTOML(l.dir+"/runtime.toml", rt)
}

// SetCronChangeHandler registers a callback invoked whenever cron.toml changes.
// Must be called before Watch.
func (l *Loader) SetCronChangeHandler(fn func(CronConfig)) {
	l.onCronChange = fn
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
			if isCronEvent(event) {
				l.reloadCron()
				continue
			}
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

// reloadCron reads cron.toml and fires the onCronChange callback.
func (l *Loader) reloadCron() {
	if l.onCronChange == nil {
		return
	}
	cfg, err := LoadCronConfig(l.dir)
	if err != nil {
		l.logger.Error("cron config reload failed", "err", err)
		return
	}
	l.onCronChange(cfg)
}

// loadRuntimeTOML parses runtime.toml. A missing file is silently treated as
// an empty RuntimeConfig (no overrides).
func loadRuntimeTOML(path string) (RuntimeConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return RuntimeConfig{}, nil
	}
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read %s: %w", path, err)
	}
	var rt RuntimeConfig
	if _, err := toml.Decode(string(data), &rt); err != nil {
		return RuntimeConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return rt, nil
}

// writeRuntimeTOML atomically writes rt to path (write to .tmp, then rename).
func writeRuntimeTOML(path string, rt RuntimeConfig) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(rt); err != nil {
		return fmt.Errorf("encode runtime.toml: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}

// mergeRuntime returns a copy of static with non-empty RuntimeConfig fields
// applied on top.
func mergeRuntime(static AgentConfig, rt RuntimeConfig) AgentConfig {
	merged := static
	if rt.LLM.Model != "" {
		merged.LLM.Model = rt.LLM.Model
	}
	return merged
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
		strings.HasSuffix(name, "runtime.toml") ||
		strings.HasSuffix(name, "SOUL.md") ||
		strings.HasSuffix(name, "IDENTITY.md") ||
		strings.HasSuffix(name, "cron.toml")
}

func isCronEvent(e fsnotify.Event) bool {
	return strings.HasSuffix(e.Name, "cron.toml")
}
