# LLM & Adapter Presets

## Goal

Flatten LLM and adapter configuration with a presets pattern:
- **Presets**: static, well-known configurations stored as code
- **Inline copy**: preset values copied directly into agent.toml at creation
- **Runtime secrets**: config block for env-var expansion (no preset reference)
- **Unified model**: same pattern for LLM backends and channel adapters

## Problem

Current design has:
- `backend: "minimax"` → looks up preset at runtime → merges with fields in agent.toml
- `auth_profile: "anthropic"` → separate credentials.toml lookup
- Adapter config scattered, inconsistent naming

This creates:
- Runtime preset lookup (complex, error-prone)
- Two lookup mechanisms (preset + credentials)
- Field name inconsistency (`auth_profile` vs `secret = { api_key = "$VAR" }`)

## Proposed Model

### Preset Definition

Presets are static Go structs with well-known configurations:

```go
// llm/preset.go
type LLMPreset struct {
    Name      string            // "minimax", "anthropic", etc. (for selection)
    Backend   string            // protocol: "openai" or "anthropic"
    BaseURL   string            // API endpoint
    Config    map[string]any    // backend-specific defaults (model, max_tokens, etc.)
}

type AdapterPreset struct {
    Name   string              // "telegram", "discord", etc.
    Config map[string]any      // adapter-specific defaults
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
            "model":      "MiniMax-Text-01",
            "max_tokens": 4096,
        },
    },
    {
        Name:    "minimax-cn",
        Backend: "anthropic",
        BaseURL: "https://api.minimaxi.com/anthropic",
        Config: map[string]any{
            "model":      "MiniMax-Text-01",
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

### Agent Config (Inline Copy)

At creation, ctl copies preset values **inline** into agent.toml. No preset reference remains:

```toml
# agent.toml — created by `zlaw ctl create --preset minimax`

[llm]
backend = "anthropic"              # from preset
base_url = "https://api.minimax.io/anthropic"  # from preset

[llm.config]                        # runtime config, expanded at spawn
api_key = "$MINIMAX_API_KEY"        # user-provided via env var
```

The `config` block contains **env-var references** (not actual values). Expansion happens inside the agent process at runtime:

```toml
[llm.config]
api_key = "$MINIMAX_API_KEY"
```

**Expansion flow:**
1. ctl reads secrets.toml
2. ctl injects env vars into agent process at spawn: `MINIMAX_API_KEY=sk-xxx`
3. Agent reads `agent.toml`
4. Agent's LLM factory expands `$MINIMAX_API_KEY` → value from its own env vars

| Syntax | Behavior |
|--------|----------|
| `$VAR` | Replace with env var value |
| `${VAR}` | Same, safe for adjacent chars |
| `$$VAR` | Literal `$VAR` (escape) |

### Config Keys by Backend

| Backend | Config Keys |
|---------|------------|
| `anthropic` | `api_key` |
| `openai` | `api_key`, `organization` |
| `openai-compatible` | `api_key` |
| `ollama` | (no config, local) |

### Adapter Config

```toml
[[adapter]]
backend = "telegram"              # from preset

[adapter.config]                  # runtime config
bot_token = "$TELEGRAM_BOT_TOKEN"  # user-provided
```

## Bootstrap Flow

When creating a new agent (`zlaw ctl create <name> --preset minimax`):

```
1. User specifies preset name: --preset minimax
2. ctl looks up preset from LLMPresets
3. ctl copies preset fields to agent.toml:
   
   [llm]
   backend = "anthropic"
   base_url = "https://api.minimax.io/anthropic"
   # config block left empty for user
   
4. User edits agent.toml, adds to config:
   
   [llm.config]
   api_key = "$MINIMAX_API_KEY"
   
5. User runs `zlaw auth add --name MINIMAX_API_KEY`
6. User adds env_vars mapping in zlaw.toml

**Expansion happens inside the agent**, not in ctl:

1. ctl injects env vars at spawn: `MINIMAX_API_KEY=sk-xxx`
2. Agent's LLM factory reads `agent.toml`
3. Factory expands `$VAR` references using `os.ExpandEnv()` or equivalent

## Changes from Current

| Old | New |
|-----|-----|
| `backend: "minimax"` (runtime lookup) | `backend: "anthropic"` (copied inline) |
| `auth_profile: "anthropic"` | `config = { api_key = "$VAR" }` |
| Separate credentials.toml lookup | Inline env-var expansion |
| Adapter `auth_profile` | Adapter `config` with env vars |
| `secret = { api_key = "$VAR" }` | `config = { api_key = "$VAR" }` (unified) |

## Benefits

1. **No runtime lookup** — preset values baked into agent.toml
2. **Single lookup** — no separate credentials.toml for auth
3. **Consistent naming** — all configs use `config` block
4. **Clear separation** — preset defines defaults, config handles secrets
5. **Static list** — presets in code, no external file to manage
6. **Portable agents** — agent.toml is self-contained

## Implementation Notes

### Preset Lookup at Creation

```go
func (c *CtlCreateCmd) Run() error {
    // Find preset by name
    preset, err := llm.FindPreset(c.Preset)
    if err != nil {
        return err
    }
    
    // Generate agent.toml with preset values copied inline
    cfg := generateAgentConfig(preset)
    
    // Write to agentHome/agent.toml
    // config block is empty, user fills in api_key
}
```

### Config Block Expansion

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

### Example: Telegram Agent

```toml
# agent.toml

[llm]
backend = "anthropic"
base_url = "https://api.minimax.io/anthropic"
model = "MiniMax-Text-01"

[llm.config]
api_key = "$MINIMAX_API_KEY"

[[adapter]]
backend = "telegram"

[adapter.config]
bot_token = "$TELEGRAM_BOT_TOKEN"
```

## Files Affected

| File | Change |
|------|--------|
| `internal/llm/preset.go` | Add LLMPreset struct, expand preset list |
| `internal/adapter/preset.go` | New file for AdapterPreset |
| `internal/config/config.go` | Update LLMConfig: add Config map |
| `cmd/zlaw/ctl.go` | Bootstrap from preset on `ctl create`, inline copy |
| `cmd/zlaw/init.go` | Update templates to use new format |
| `docs/design/llm_presets.md` | This document |
| `docs/design/channel_adapter.md` | Update adapter config section |

## Open Questions

1. **Config block vs top-level?** Current design puts secrets in `config` sub-block. Alternative: top-level fields like `api_key = "$VAR"`. Tradeoff: config block is explicit, top-level is simpler.

2. **Env var validation?** Warn if `$VAR` in config but no mapping in zlaw.toml env_vars? Or let it fail at spawn?

3. **Preset updates?** If a preset changes (e.g., new base_url), existing agents keep old values. Document this or add migration tooling?

4. **Config schema validation?** Need to validate config keys match backend expectation. Backend-specific validators?

## See Also

- [agent_secrets.md](./agent_secrets.md) — secrets and env var injection
- [agent_lifecycle.md](./agent_lifecycle.md) — agent spawning
- [channel_adapter.md](./channel_adapter.md) — adapter architecture