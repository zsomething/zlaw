# Agent: Secrets

## Overview

Secrets are managed by the human operator via `zlaw auth` CLI. Stored in `secrets.toml` (formerly `credentials.toml`). At spawn time, ctl injects secret values as environment variables into the agent process. Hub has no credential access.

## Ownership Model

| Component | Role |
|-----------|------|
| **ctl (operator)** | Owns `secrets.toml`, manages via `zlaw auth` |
| **Hub** | Message routing only. No secret access. |
| **Agent** | Receives only env vars, never sees file path |

## Design Goals

1. **Secret isolation** — agent receives only env vars with values it needs
2. **No secret file exposure** — no file path exposed to agent
3. **Operator control** — human operator manages secrets via CLI
4. **Minimal surface** — agent receives only the secrets it needs

## Injection Flow

```
Human Operator (ctl)                    ctl                              Agent
       │                                  │                                 │
       │ Edit secrets.toml:                │                                 │
       │ MINIMAX_API_KEY_DEV = "xxx"      │                                 │
       │ ANTHROPIC_API_KEY = "sk-..."      │                                 │
       │                                  │                                 │
       │ Configure in zlaw.toml:           │                                 │
       │ [[agents]]                       │                                 │
       │ id = "assistant"                  │                                 │
       │ env_vars = [                     │                                 │
       │   { name = "MINIMAX_API_KEY",    │                                 │
       │     from_secret = "MINIMAX_API_KEY_DEV" } ] │                       │
       │                                  │                                 │
       │                                  │ At spawn:                       │
       │                                  │ Read secrets.toml               │
       │                                  │ Look up MINIMAX_API_KEY_DEV     │
       │                                  │                                 │
       │                                  │ Inject as env vars:             │
       │                                  │ MINIMAX_API_KEY=xxx ────────────┼── Agent sees only
       │                                  │                                  │   env vars
       │                                  │ Agent reads env vars             │  No file path
       │                                  │ $MINIMAX_API_KEY → value        │  No file to read
```

## Secrets File Location

Global secrets file managed by ctl:

```
$ZLAW_HOME/
├── zlaw.toml           # hub + agents config
└── secrets.toml        # operator-managed, ctl reads at spawn
```

Agent directories do **not** contain secrets. Agent cannot read its own secrets file.

## Agent Config (Presets + Inline Copy)

In `agent.toml`, agents use presets with inline copy. Preset values (backend, client_config, model_config, etc.) are copied at creation:

```toml
[llm]
backend = "anthropic"
client_config = {
  base_url = "https://api.minimax.io/anthropic",
  api_key = "$MINIMAX_API_KEY"
}
model = "MiniMax-Text-01"
model_config = {
  max_tokens = 4096,
  prompt_caching = true
}

[[adapter]]
backend = "telegram"
client_config = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

[[adapter]]
backend = "telegram"
config = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

No values in agent.toml — only env var references (`$VAR_NAME`). Values injected by ctl at spawn.

See [llm_presets.md](./llm_presets.md) for the presets pattern.

## Secrets Structure

Stored in `secrets.toml` (ctl-managed):

```toml
MINIMAX_API_KEY_DEV = "xxx"
MINIMAX_API_KEY_PROD = "yyy"
ANTHROPIC_API_KEY = "sk-ant-..."
TELEGRAM_BOT_TOKEN = "12345:abc..."
FIZZY_API_KEY = "..."
```

Flat key-value pairs. Supports multiple credentials of same service (e.g., `MINIMAX_API_KEY_DEV` vs `MINIMAX_API_KEY_PROD`).

## Agent Mapping

In `zlaw.toml`:

```toml
[[agents]]
id = "assistant"
executor = "subprocess"
env_vars = [
  { name = "MINIMAX_API_KEY", from_secret = "MINIMAX_API_KEY_DEV" },
  { name = "TELEGRAM_BOT_TOKEN", from_secret = "TELEGRAM_BOT_TOKEN" }
]
```

| Field | Description |
|-------|-------------|
| `name` | Env var name injected to agent |
| `from_secret` | Key in secrets.toml |

## CLI Commands

```bash
# Add secrets
zlaw auth add --name MINIMAX_API_KEY_DEV
# Prompts for value, saves to secrets.toml

# List secrets (names only, no values)
zlaw auth list

# Remove secret
zlaw auth remove --name MINIMAX_API_KEY_DEV
```

## Security Properties

- **No file to read** — secrets as env vars, not file path
- **No enumeration** — agent doesn't know other secret names
- **Compromise resistant** — even if agent is prompt-injected, no file path to read
- **Minimal exposure** — only needed secrets injected
- **Subprocess filtered** — secret env vars not passed to subprocesses (e.g., bash tool)
- **Operator control** — human operator manages via CLI, not agent

## See Also

- [agent_lifecycle.md](./agent_lifecycle.md) — agent lifecycle
- [security.md](./security.md) — security model
- [command_line.md](./command_line.md) — CLI reference