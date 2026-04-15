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
	Agent   AgentMeta               `toml:"agent"`
	LLM     LLMConfig               `toml:"llm"`
	Context ContextConfig           `toml:"context"`
	Tools   ToolsConfig             `toml:"tools"`
	Adapter []AdapterInstanceConfig `toml:"adapter"`
	Sticky  StickyConfig            `toml:"sticky"`
	Serve   ServeConfig             `toml:"serve"`
	Memory  MemoryConfig            `toml:"memory"`
}

// MemoryConfig controls long-term memory behaviour.
type MemoryConfig struct {
	// Embedder configures the embedding backend for semantic (vector) search.
	// When Backend is empty the store falls back to keyword matching.
	Embedder EmbedderConfig `toml:"embedder"`
}

// EmbedderConfig selects and configures the embedding backend used by the
// semantic memory store. Authentication is handled via credentials.toml,
// the same as the LLM backend.
type EmbedderConfig struct {
	// Backend selects a named preset (e.g. "minimax-openai", "openrouter").
	// The preset must resolve to an OpenAI-compatible endpoint.
	Backend string `toml:"backend"`

	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string `toml:"model"`

	// BaseURL overrides the endpoint URL from the preset. Leave empty to use the
	// preset default.
	BaseURL string `toml:"base_url"`

	// AuthProfile is the credentials.toml profile name. Leave empty to use the
	// same profile as the LLM backend.
	AuthProfile string `toml:"auth_profile"`
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
	// ID is the stable, machine-readable identifier for the agent. It is used
	// for all runtime paths (socket, pid, session dir, memory dir) and as the
	// NATS username when connecting to the hub. It must not change at runtime.
	ID string `toml:"id"`
	// Name is the optional human-readable display name used in system prompts
	// and agent greetings. Falls back to ID if unset.
	Name        string `toml:"name"`
	Description string `toml:"description"`
	// Roles are the functional specializations of this agent.
	// Used by hub registry and manager routing for peer discovery.
	// Example: roles = ["calendar", "scheduling"]
	Roles []string `toml:"roles"`
	// Manager is deprecated; all agents have equal P2P permissions (#273).
	// Kept for config compatibility during migration; has no runtime effect.
	Manager bool `toml:"manager"`
}

// DisplayName returns the human-readable display name. When Name is empty it
// falls back to ID, so callers never need to handle the empty-name case.
func (m AgentMeta) DisplayName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
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

// AdapterInstanceConfig configures a single adapter instance.
type AdapterInstanceConfig struct {
	// Type is the adapter type (e.g., "telegram", "fizzy", "cli").
	Type string `toml:"type"`
	// AuthProfile is the credentials.toml profile name for this adapter.
	// The profile's Data map contains adapter-specific keys (e.g.,
	// "telegram_bot_token", "fizzy_api_key").
	AuthProfile string `toml:"auth_profile"`
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
	dir          string // agent.toml directory (hub-owned)
	workspace    string // workspace directory (SOUL.md, IDENTITY.md); agent has access
	onChange     func(AgentConfig, Personality)
	onCronChange func(CronConfig)
	watcher      *fsnotify.Watcher
	logger       *slog.Logger

	mu          sync.Mutex
	staticCfg   AgentConfig // from agent.toml; set once in Load, never mutated
	personality Personality // from SOUL.md / IDENTITY.md; updated on each Load
}

// NewLoader creates a Loader for the given directories.
// agentDir contains agent.toml (hub-owned); workspace contains SOUL.md and IDENTITY.md.
// When workspace is empty, personality files are loaded from agentDir for
// backward compatibility.
func NewLoader(agentDir string, workspace string, onChange func(AgentConfig, Personality), logger *slog.Logger) (*Loader, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	return &Loader{dir: agentDir, workspace: workspace, onChange: onChange, watcher: w, logger: logger}, nil
}

// Load reads agent.toml, runtime.toml from agentDir, and SOUL.md/IDENTITY.md
// from the workspace directory, merges the runtime overrides, and returns the result.
// It also caches the static config and personality for use by ReloadRuntime.
func (l *Loader) Load() (AgentConfig, Personality, error) {
	static, err := loadTOML(l.dir + "/agent.toml")
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}

	// Use workspace dir for personality if set, otherwise fall back to agent dir.
	personalityDir := l.workspace
	if personalityDir == "" {
		personalityDir = l.dir
	}

	p, err := loadPersonality(personalityDir)
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

// WriteRuntimeFieldToDir updates a runtime-configurable field in the
// runtime.toml located in agentDir. It validates the key against the allowlist.
// This is the hub-side equivalent of Loader.WriteRuntimeField for cases where
// no Loader instance is available (e.g. hub management API).
func WriteRuntimeFieldToDir(agentDir, key, value string) error {
	if _, ok := runtimeFieldAllowlist[key]; !ok {
		return fmt.Errorf("config: field %q is not runtime-configurable", key)
	}

	existing, err := loadRuntimeTOML(agentDir + "/runtime.toml")
	if err != nil {
		return fmt.Errorf("config: read runtime.toml before write: %w", err)
	}

	rt := existing
	switch key {
	case "llm.model":
		rt.LLM.Model = value
	default:
		return fmt.Errorf("config: field %q is not runtime-configurable", key)
	}

	return writeRuntimeTOML(agentDir+"/runtime.toml", rt)
}

// SetCronChangeHandler registers a callback invoked whenever cron.toml changes.
// Must be called before Watch.
func (l *Loader) SetCronChangeHandler(fn func(CronConfig)) {
	l.onCronChange = fn
}

// Watch starts watching the agent directory and workspace (if set) for changes.
// It blocks until ctx is cancelled. Call in a goroutine.
func (l *Loader) Watch(ctx context.Context) error {
	if err := l.watcher.Add(l.dir); err != nil {
		return fmt.Errorf("watch dir %s: %w", l.dir, err)
	}
	if l.workspace != "" && l.workspace != l.dir {
		if err := l.watcher.Add(l.workspace); err != nil {
			return fmt.Errorf("watch workspace %s: %w", l.workspace, err)
		}
	}
	defer l.watcher.Close() //nolint:errcheck

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

// LoadAgentConfigFile reads and parses an agent.toml file at path,
// expanding ${ENV_VAR} references in string values.
func LoadAgentConfigFile(path string) (AgentConfig, error) {
	return loadTOML(path)
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
