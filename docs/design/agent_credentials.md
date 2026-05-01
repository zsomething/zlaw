# Agent: Credentials

## Overview

Credentials are managed by the human operator via `zlaw auth` CLI. The hub **does not** own or manage credentials. At spawn time, the hub reads from the global credentials file and injects values as environment variables into the agent process.

## Ownership Model

| Component | Role |
|-----------|------|
| **ctl (operator)** | Owns `credentials.toml`, manages via `zlaw auth` |
| **Hub** | Reads credentials at spawn, injects as env vars |
| **Agent** | Receives only env vars, never sees file path |

## Design Goals

1. **Secret isolation** — agent only receives env vars with values it needs
2. **No credential file exposure** — no file path exposed to agent
3. **Operator control** — human operator manages credentials via CLI
4. **Minimal surface** — agent receives only the credentials it needs

## Injection Flow

```
Human Operator (ctl)                    Hub                              Agent
       │                                  │                                 │
       │ Edit credentials.toml:            │                                 │
       │ [anthropic]                      │                                 │
       │   api_key = "sk-ant-..."        │                                 │
       │                                  │                                 │
       │                                  │ At spawn:                       │
       │                                  │ Read credentials.toml            │
       │                                  │ Extract needed profiles          │
       │                                  │                                  │
       │                                  │ Inject as env vars:             │
       │                                  │   ANTHROPIC_API_KEY=sk-ant-... ──┤── Agent sees only
       │                                  │   (per-profile keys as env vars)   │   env vars
       │                                  │                                  │
       │                                  │ Agent reads env vars             │  No file path
       │                                  │   apiKey := os.Getenv(...)         │  No file to read
```

## Credentials File Location

Global credentials file managed by ctl:

```
$ZLAW_HOME/
├── zlaw.toml           # hub + agents config
└── credentials.toml    # operator-managed, hub reads at spawn
```

Agent directories do **not** contain credentials. Agent cannot read its own credentials file.

## Environment Variables

Hub injects profile values as env vars at spawn. Env var names are derived from profile name and key:

| Profile | Env Var | Example |
|---------|---------|---------|
| `anthropic` | `ANTHROPIC_API_KEY` | `sk-ant-...` |
| `minimax` | `MINIMAX_API_KEY` | `sk-...` |
| `telegram` | `TELEGRAM_BOT_TOKEN` | `12345:abc...` |
| `fizzy` | `FIZZY_API_KEY` | `...` |

## Profile Reference

In `agent.toml`:

```toml
[llm]
backend = "anthropic"
auth_profile = "anthropic"
model = "claude-sonnet-4-5"

[[adapter]]
type = "telegram"
auth_profile = "telegram"
```

The hub reads the `auth_profile` from `agent.toml` and injects the corresponding env vars at spawn.

## Credential Structure

Stored in `credentials.toml` (ctl-managed):

```toml
[profiles.anthropic]
name = "anthropic"
data = { api_key = "sk-ant-..." }

[profiles.minimax]
name = "minimax"
data = { api_key = "sk-..." }

[profiles.telegram]
name = "telegram"
data = { telegram_bot_token = "12345:abc..." }

[profiles.fizzy]
name = "fizzy"
data = { fizzy_api_key = "..." }
```

## CLI Commands

```bash
# Add credentials
zlaw auth add --profile anthropic
# Prompts for API key, saves to credentials.toml

zlaw auth add --profile telegram --key 12345:abc...
# Non-interactive: specify key directly

# List profiles
zlaw auth list

# Remove profile
zlaw auth remove --profile anthropic
```

## Security Properties

- **No file to read** — credentials as env vars, not file path
- **No enumeration** — agent doesn't know other profile names
- **Compromise resistant** — even if agent is prompt-injected, no file path to read
- **Minimal exposure** — only needed credentials injected
- **Subprocess filtered** — credential env vars not passed to subprocesses (e.g., bash tool)
- **Operator control** — human operator manages via CLI, not agent

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent startup sequence
- [security.md](./security.md) — security model
- [command_line.md](./command_line.md) — CLI reference
