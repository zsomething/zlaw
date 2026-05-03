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
// semantic memory store.
type EmbedderConfig struct {
	// Backend is the wire protocol ("openai" or "anthropic").
	Backend string `toml:"backend"`

	// ClientConfig holds config for client construction (base_url, api_key).
	// Env-var references ($VAR) expanded at load time.
	ClientConfig map[string]any `toml:"client_config"`

	// Model is the embedding model name (e.g. "text-embedding-3-small").
	Model string `toml:"model"`
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
	// DisplayName is the optional human-readable display name used in system
	// prompts and agent greetings. Falls back to ID if unset.
	DisplayName string `toml:"name"`
	Description string `toml:"description"`
	// Roles are the functional specializations of this agent.
	// Used by hub registry and peer discovery.
	// Example: roles = ["calendar", "scheduling"]
	Roles []string `toml:"roles"`
}

// GetDisplayName returns the human-readable display name. When Name is empty it
// falls back to ID, so callers never need to handle the empty-name case.
func (m AgentMeta) GetDisplayName() string {
	if m.DisplayName != "" {
		return m.DisplayName
	}
	return m.ID
}

// LLMConfig holds LLM backend settings.
// No secrets in agent.toml — use env-var references ($VAR) in client_config.
type LLMConfig struct {
	// Backend is the wire protocol ("openai" or "anthropic").
	Backend string `toml:"backend"`

	// ClientConfig holds config for client construction (base_url, api_key).
	// Env-var references ($VAR) expanded at load time.
	ClientConfig map[string]any `toml:"client_config"`

	// Model is the model name (e.g., "claude-sonnet-4-20250514").
	Model string `toml:"model"`

	// ModelConfig holds provider-specific behavior defaults.
	// Includes max_tokens, timeout_sec, prompt_caching, etc.
	ModelConfig map[string]any `toml:"model_config"`

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
	// "drop_pairs". Empty defaults to ["drop_pairs"].
	ContextPruneLevels []string `toml:"context_prune_levels"`

	// ContextSummarizeModel is the model to use for summarization. When set,
	// summarization uses this model instead of the main model.
	ContextSummarizeModel string `toml:"context_summarize_model"`

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
	// Backend is the adapter protocol (e.g., "telegram", "fizzy", "slack").
	Backend string `toml:"backend"`
	// ClientConfig holds adapter-specific config (e.g., bot_token, api_key).
	// Env-var references ($VAR) expanded at load time.
	ClientConfig map[string]any `toml:"client_config"`
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
	configFile   string // path to agent config file (owned by ctl); e.g., $ZLAW_HOME/agent-{id}.toml
	dir          string // agent runtime directory (SOUL.md, IDENTITY.md, sessions, etc.)
	workspace    string // workspace directory (SOUL.md, IDENTITY.md); agent has access
	onChange     func(AgentConfig, Personality)
	onCronChange func(CronConfig)
	watcher      *fsnotify.Watcher
	logger       *slog.Logger

	mu          sync.Mutex
	staticCfg   AgentConfig // from configFile; set once in Load, never mutated
	personality Personality // from SOUL.md / IDENTITY.md; updated on each Load
}

// NewLoader creates a Loader for the given directories.
// configFile is the path to the agent config file (owned by ctl).
// By convention: $ZLAW_HOME/agent-{id}.toml
// agentDir contains runtime data (SOUL.md, IDENTITY.md, sessions, etc.)
// workspace contains SOUL.md and IDENTITY.md; agent has access.
// When workspace is empty, personality files are loaded from agentDir for
// backward compatibility.
func NewLoader(configFile string, agentDir string, workspace string, onChange func(AgentConfig, Personality), logger *slog.Logger) (*Loader, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	return &Loader{configFile: configFile, dir: agentDir, workspace: workspace, onChange: onChange, watcher: w, logger: logger}, nil
}

// Load reads agent.toml, runtime.toml from agentDir, and SOUL.md/IDENTITY.md
// from the workspace directory, merges the runtime overrides, and returns the result.
// It also caches the static config and personality for use by ReloadRuntime.
func (l *Loader) Load() (AgentConfig, Personality, error) {
	static, err := loadTOML(l.configFile)
	if err != nil {
		return AgentConfig{}, Personality{}, err
	}

	// Use ZLAW_AGENT_HOME for personality if set, otherwise fall back to agent dir.
	personalityDir := AgentHome()
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

// SetOnChange replaces the onChange callback. It is safe to call
// concurrently.
func (l *Loader) SetOnChange(fn func(AgentConfig, Personality)) {
	l.mu.Lock()
	l.onChange = fn
	l.mu.Unlock()
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
	// Expand env vars in nested Config maps.
	cfg.LLM.ClientConfig = expandConfigMap(cfg.LLM.ClientConfig)
	cfg.LLM.ModelConfig = expandConfigMap(cfg.LLM.ModelConfig)
	for i := range cfg.Adapter {
		cfg.Adapter[i].ClientConfig = expandConfigMap(cfg.Adapter[i].ClientConfig)
	}
	cfg.Memory.Embedder.ClientConfig = expandConfigMap(cfg.Memory.Embedder.ClientConfig)
	return cfg, nil
}

// expandConfigMap recursively expands $VAR and ${VAR} references in config values.
func expandConfigMap(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	result := make(map[string]any, len(cfg))
	for k, v := range cfg {
		switch val := v.(type) {
		case string:
			result[k] = os.ExpandEnv(val)
		case map[string]any:
			result[k] = expandConfigMap(val)
		default:
			result[k] = v
		}
	}
	return result
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
