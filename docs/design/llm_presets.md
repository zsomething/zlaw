# LLM & Adapter Presets (Inline Copy Pattern)

## Pattern

Presets are **templates for inline copying** into `agent.toml` at creation. No runtime lookup after copy.

```toml
# agent.toml — copied from preset at creation, user edits directly after

[llm]
backend = "anthropic"
client_config = { base_url = "...", api_key = "$API_KEY" }
model = "claude-sonnet-4-20250514"
model_config = { max_tokens = 8192, prompt_caching = true }

[[adapter]]
backend = "telegram"
client_config = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

## Goal

Flatten LLM and adapter configuration with an inline copy pattern:
- **Presets**: static, well-known configurations stored as code
- **Inline copy**: preset values copied directly into agent.toml at creation
- **Clear separation**: client_config (auth/endpoint), model (identity), model_config (behavior)
- **Unified model**: same pattern for LLM backends and channel adapters

## Problem

Current design has:
- `backend: "minimax"` → looks up preset at runtime → merges with fields in agent.toml
- `auth_profile: "anthropic"` → separate credentials.toml lookup
- Adapter config scattered, inconsistent naming

This creates:
- Runtime preset lookup (complex, error-prone)
- Two lookup mechanisms (preset + credentials)
- Field name inconsistency

## Data Model

### LLMPreset (Code)

Static presets bundle the protocol with default values:

```go
type LLMPreset struct {
    Name         string            // "minimax", "anthropic" (for selection)
    Backend      string            // protocol: "openai" or "anthropic"
    ClientConfig map[string]any    // auth/endpoint: base_url, api_key (secrets excluded)
    ModelConfig  map[string]any    // provider-specific behavior: max_tokens, prompt_caching
    DefaultModel string            // model name
}
```

### LLMConfig (TOML)

```go
type LLMConfig struct {
    Backend      string            `toml:"backend"`       // "openai" or "anthropic"
    ClientConfig map[string]any   `toml:"client_config"` // auth/endpoint
    Model        string           `toml:"model"`         // model name
    ModelConfig  map[string]any   `toml:"model_config"`   // provider-specific behavior
}
```

### TOML Structure

```toml
[llm]
# Client construction (from preset)
backend = "anthropic"
client_config = {
  base_url = "https://api.anthropic.com",
  api_key = "$ANTHROPIC_API_KEY"
}

# Model (from preset, user can change)
model = "claude-sonnet-4-20250514"

# Model behavior (from preset, user can change)
model_config = {
  max_tokens = 8192,
  timeout_sec = 120,
  prompt_caching = true
}
```

### Preset List (Static)

```go
var presets = map[string]LLMPreset{
    "minimax": {
        Name:    "minimax",
        Backend: "anthropic",
        ClientConfig: map[string]any{
            "base_url": "https://api.minimax.io/anthropic",
        },
        ModelConfig: map[string]any{
            "max_tokens":     4096,
            "timeout_sec":     60,
            "prompt_caching":  true,
        },
        DefaultModel: "MiniMax-Text-01",
    },
    "anthropic": {
        Name:    "anthropic",
        Backend: "anthropic",
        ClientConfig: map[string]any{
            "base_url": "https://api.anthropic.com",
        },
        ModelConfig: map[string]any{
            "max_tokens":     8192,
            "timeout_sec":    120,
            "prompt_caching": true,
        },
        DefaultModel: "claude-sonnet-4-20250514",
    },
    "openai": {
        Name:    "openai",
        Backend: "openai",
        ClientConfig: map[string]any{
            "base_url": "https://api.openai.com/v1",
        },
        ModelConfig: map[string]any{
            "max_tokens":  4096,
            "timeout_sec": 60,
        },
        DefaultModel: "gpt-4o",
    },
}
```

## Backend-Specific Model Config Keys

| Backend | Model Config Keys |
|---------|-------------------|
| `anthropic` | `max_tokens`, `timeout_sec`, `prompt_caching` |
| `openai` | `organization`, `max_tokens`, `timeout_sec` |

Validation at load time.

## Adapter Presets

```go
type AdapterPreset struct {
    Name         string            // "telegram", "slack"
    Backend      string            // protocol
    ClientConfig map[string]any    // adapter-specific (token, etc.)
}
```

```toml
[[adapter]]
backend = "telegram"
client_config = {
  bot_token = "$TELEGRAM_BOT_TOKEN"
}
```

## Expansion Flow

```
1. ctl reads secrets.toml
2. ctl injects env vars into agent process at spawn
3. Agent reads agent.toml
4. loadTOML() expands $VAR using os.ExpandEnv()
5. expandClientConfig() recursively expands nested maps
6. LLM factory extracts keys for the selected backend
```

| Syntax | Behavior |
|--------|----------|
| `$VAR` | Replace with env var value |
| `${VAR}` | Same, safe for adjacent chars |
| `$$VAR` | Literal `$VAR` (escape) |

## Changes from Current

| Old | New |
|-----|-----|
| `backend: "minimax"` (runtime lookup) | `backend: "anthropic"` (copied inline) |
| `auth_profile: "anthropic"` | `client_config = { api_key = "$VAR" }` |
| Separate credentials.toml lookup | Inline env-var expansion |
| Preset had `BaseURL`, `APIFormat` fields | Preset has `ClientConfig` + `ModelConfig` + `DefaultModel` |
| No model_config | `model_config` for provider-specific behavior |

## Benefits

1. **No runtime lookup** — preset values baked into agent.toml
2. **Clear separation** — client_config (auth), model (identity), model_config (behavior)
3. **Backend-appropriate** — model_config varies per provider
4. **Static list** — presets in code, no external file to manage
5. **Portable agents** — agent.toml is self-contained

## Files Affected

| File | Change |
|------|--------|
| `internal/llm/preset.go` | `LLMPreset.ClientConfig`, `LLMPreset.ModelConfig`, `LLMPreset.DefaultModel` |
| `internal/llm/presets.go` | Preset definitions with new fields |
| `internal/adapter/preset.go` | `AdapterPreset.ClientConfig` |
| `internal/config/config.go` | `LLMConfig.ClientConfig`, `LLMConfig.Model`, `LLMConfig.ModelConfig` |
| `internal/llm/factory.go` | Backend-specific FromConfig functions |
| `cmd/zlaw/ctl.go` | Inline copy template with new structure |
| `docs/design/llm_presets.md` | This document |