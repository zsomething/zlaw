# LLM & Adapter Presets

## Goal

Flatten LLM and adapter configuration with a presets pattern:
- **Presets**: static, well-known configurations stored as code
- **Config copy**: preset values copied to agent config at creation
- **Runtime overrides**: agent.toml can override preset defaults
- **Unified model**: same pattern for LLM backends and channel adapters

## Problem

Current design has:
- `backend: "minimax"` → looks up preset → merges with fields in agent.toml
- `auth_profile: "anthropic"` → separate credentials.toml lookup
- Adapter config scattered, inconsistent naming

This creates:
- Two lookup mechanisms (preset + credentials)
- Field name inconsistency (`auth_profile` vs `secret = { api_key = "$VAR" }`)
- Complicated merge logic in factory

## Proposed Model

### Preset Definition

Presets are static Go structs with well-known configurations:

```go
// llm/preset.go
type LLMPreset struct {
    Name      string            // "minimax", "anthropic", etc.
    Backend   string            // protocol: "openai" or "anthropic"
    BaseURL   string            // API endpoint
    Config    map[string]any    // backend-specific defaults
}

type AdapterPreset struct {
    Name   string         // "telegram", "discord", "slack"
    Config map[string]any  // adapter-specific defaults
}
```

### Preset List (Static)

```go
// llm/presets.go
var LLMPresets = []LLMPreset{
    {
        Name:    "minimax",
        Backend: "anthropic",
        BaseURL: "https://api.minimax.io/anthropic",
        Config: map[string]any{
            "model":     "MiniMax-Text-01",
            "max_tokens": 4096,
        },
    },
    {
        Name:    "minimax-cn",
        Backend: "anthropic",
        BaseURL: "https://api.minimaxi.com/anthropic",
        Config: map[string]any{
            "model":     "MiniMax-Text-01",
            "max_tokens": 4096,
        },
    },
    {
        Name:    "anthropic",
        Backend: "anthropic",
        BaseURL: "https://api.anthropic.com",
        Config: map[string]any{
            "model": "claude-sonnet-4-20250514",
        },
    },
    {
        Name:    "openai",
        Backend: "openai",
        BaseURL: "https://api.openai.com/v1",
        Config: map[string]any{
            "model": "gpt-4o",
        },
    },
    // ... more presets
}

var AdapterPresets = []AdapterPreset{
    {
        Name: "telegram",
        Config: map[string]any{
            "parse_mode": "Markdown",
        },
    },
    {
        Name: "slack",
        Config: map[string]any{
            "reaction": true,
        },
    },
}
```

### Agent Config (Flat)

Agent.toml uses flat config with env-var expansion:

```toml
[llm]
preset = "minimax"                    # name from LLMPresets
model = "MiniMax-Text-01"             # override preset
timeout_sec = 120                     # override preset

# Config block — keys match backend convention:
# - api_key / token / api_token (varies by provider)
# - base_url (optional, overrides preset)
config = {
    api_key = "$MINIMAX_API_KEY",      # env var expansion
}
```

### Config Keys by Backend

| Backend | Config Keys |
|---------|------------|
| `anthropic` | `api_key`, `base_url` |
| `openai` | `api_key`, `base_url`, `organization` |
| `openai-compatible` | `api_key`, `base_url` |
| `ollama` | `base_url` (no api_key) |

Adapter config follows same pattern:

```toml
[[adapter]]
preset = "telegram"

config = {
    bot_token = "$TELEGRAM_BOT_TOKEN",
}
```

## Bootstrap Flow

When creating a new agent (`zlaw ctl create <name>`):

```
1. ctl reads preset list from LLMPresets
2. User selects preset (or specifies name)
3. ctl copies preset config to agent.toml:
   
   [llm]
   preset = "minimax"
   config = {
       # Keys from preset.Config, values are env-var placeholders
       # User fills in api_key via zlaw auth
   }
   
4. User runs `zlaw auth add --name MINIMAX_API_KEY`
5. User adds env_vars mapping in zlaw.toml

6. At spawn:
   - ctl reads config block
   - Expands env vars: "$MINIMAX_API_KEY" → actual value
   - Injects as env vars to agent process
```

## Env Var Expansion

Config values support `${VAR}` or `$VAR` syntax:

```toml
config = {
    api_key = "$MINIMAX_API_KEY",        # expands at spawn
    base_url = "$CUSTOM_ENDPOINT",       # optional
}
```

| Syntax | Behavior |
|--------|----------|
| `$VAR` | Replace with env var value |
| `${VAR}` | Same, safe for adjacent chars |
| `$$VAR` | Literal `$VAR` (escape) |
| `"$UNKNOWN"` | Empty string if not set |

## Changes from Current

| Old | New |
|-----|-----|
| `backend: "minimax"` | `preset: "minimax"` |
| `auth_profile: "anthropic"` | Config block with `api_key = "$VAR"` |
| Separate credentials.toml lookup | Inline env-var expansion |
| Adapter `auth_profile` | Adapter `config` with env vars |
| `secret = { api_key = "$VAR" }` | `config = { api_key = "$VAR" }` (unified) |

## Benefits

1. **Single lookup** — no separate credentials.toml for auth
2. **Consistent naming** — all configs use `config` block
3. **Clear separation** — preset defines defaults, agent config overrides
4. **Env var everywhere** — unified injection model for LLM and adapters
5. **Static list** — presets in code, no external file to manage

## Implementation Notes

### Preset Lookup at Creation

```go
func (c *CtlCreateCmd) Run() error {
    // Find preset
    preset, err := llm.FindPreset(c.Preset)
    if err != nil {
        return err
    }
    
    // Generate agent.toml with preset config
    cfg := generateAgentConfig(preset)
    
    // Write to agentHome/agent.toml
}
```

### Runtime Expansion

```go
func expandConfig(cfg map[string]any, secrets map[string]string) map[string]any {
    result := make(map[string]any)
    for k, v := range cfg {
        if str, ok := v.(string); ok {
            result[k] = expandEnvVar(str, secrets)
        } else {
            result[k] = v
        }
    }
    return result
}
```

### Adapter Presets

```toml
# telegram agent.toml
[[adapter]]
preset = "telegram"
config = {
    bot_token = "$TELEGRAM_BOT_TOKEN",
}

# discord agent.toml  
[[adapter]]
preset = "discord"
config = {
    bot_token = "$DISCORD_BOT_TOKEN",
    guild_id = "123456",
}
```

## Files Affected

| File | Change |
|------|--------|
| `internal/llm/preset.go` | Add LLMPreset struct, expand preset list |
| `internal/adapter/preset.go` | New file for AdapterPreset |
| `internal/config/config.go` | Update LLMConfig: `Preset`, `Config` |
| `cmd/zlaw/ctl.go` | Bootstrap from preset on `ctl create` |
| `cmd/zlaw/init.go` | Update templates to use new format |
| `docs/design/llm-presets.md` | This document |
| `docs/design/channel_adapter.md` | Update adapter config section |

## Open Questions

1. **Preset file vs Go code?** Static list in Go is simpler, but external YAML/JSON allows adding presets without recompile. Decision: Go for now, file-based later if needed.

2. **Env var validation?** Warn if `$VAR` in config but no mapping in zlaw.toml env_vars? Or let it fail at spawn?

3. **Preset inheritance?** Allow presets to extend others? Probably not needed initially.

4. **Config schema validation?** Need to validate config keys match backend expectation. Backend-specific validators?

## See Also

- [agent_credentials.md](./agent_credentials.md) — secrets and env var injection
- [agent_lifecycle.md](./agent_lifecycle.md) — agent spawning
- [channel_adapter.md](./channel_adapter.md) — adapter architecture