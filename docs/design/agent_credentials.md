# Agent: Credentials

## Overview

Credentials are injected into agents at spawn time via environment variables. Each credential value is passed directly as an env var — no file path exposed. This prevents a compromised/prompt-injected agent from reading secrets.

## Design Goals

1. **Secret isolation** — agent only receives env vars with values it needs
2. **No credential file exposure** — no file path, no file to read
3. **Minimal surface** — agent receives only the credentials it needs

## Injection Flow

```
Hub                                     Agent
 │                                        │
 │ Read agents/<id>/credentials.toml      │
 │ Extract needed profiles                │
 │                                        │
 │ Inject as env vars at spawn:           │
 │   MINIMAX_API_KEY=sk-...              ──┤─ Agent only sees env vars
 │   TELEGRAM_BOT_TOKEN=...              ──┤
 │   (per-profile keys as env vars)        │
 │                                        │
 │ Agent reads env vars directly           │  No file path, no file to read
```

## Environment Variables

Hub injects profile values as env vars. Env var names are derived from profile name and key:

| Profile | Env Var | Example |
|---------|---------|---------|
| `minimax-default` | `MINIMAX_DEFAULT_API_KEY` | `sk-...` |
| `anthropic-main` | `ANTHROPIC_MAIN_API_KEY` | `sk-ant-...` |
| `telegram-bot` | `TELEGRAM_BOT_TOKEN` | `12345:abc...` |

## Agent Usage

```go
// Agent reads credential via env var lookup
apiKey := os.Getenv("MINIMAX_DEFAULT_API_KEY")
```

Agent does NOT:
- Receive any file path
- Know where credentials are stored
- Have access to credential files

## Profile Reference

In `agent.toml`:
```toml
[llm]
backend = "minimax"
auth_profile = "minimax-default"
```

The agent looks up `MINIMAX_DEFAULT_API_KEY` env var at startup.

## Credential Structure

Credentials are stored in `credentials.toml` (hub-owned):

```toml
[minimax-default]
api_key = "sk-..."

[anthropic-main]
api_key = "sk-ant-..."

[telegram-bot]
telegram_bot_token = "12345:abc..."
```

Hub extracts and injects only the profiles the agent needs.

## Security Properties

- **No file to read** — credentials as env vars, not file path
- **No enumeration** — agent doesn't know other profile names
- **Compromise resistant** — even if agent is prompt-injected, no file path to read
- **Minimal exposure** — only needed credentials injected
- **Subprocess filtered** — credential env vars not passed to subprocesses (e.g., bash tool)

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent startup sequence
- [security.md](./security.md) — security model
- [plans/separation.md](../plans/separation.md) — architectural violations