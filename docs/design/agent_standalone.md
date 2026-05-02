# Agent: Standalone Mode

## Overview

A standalone agent is a self-contained process that runs the agentic loop independently. It receives input, executes tools, and produces output. No hub dependency.

## Startup Sequence

```
1. Load agent.toml       → config (model, backend, secret references)
2. Load SOUL.md          → system prompt component (personality)
3. Load IDENTITY.md      → system prompt component (role definition)
4. Restore history       → sessions/<id>/ (JSONL files)
5. Register tools        → from tools/ dir and skills/
6. Connect to LLM        → via configured backend (Minimax, OpenRouter, Anthropic)
7. Enter loop            → read input → build context → call LLM → execute tools
```

## Filesystem

```
$ZLAW_AGENT_HOME/           # set by ZLAW_AGENT_HOME env var
├── agent.toml             # configuration (secret references, not values)
├── runtime.toml           # runtime overrides (watched, hot-reloaded)
├── cron.toml              # scheduled tasks
├── SOUL.md                # personality (hot-reload on change)
├── IDENTITY.md            # role definition (hot-reload on change)
├── skills/               # per-agent skill files
├── sessions/             # conversation history
│   └── <session-id>.jsonl # per-session turn log
├── memories/             # long-term memory
│   ├── <topic>.md        # memory files
│   └── vector.db         # semantic index (if enabled)
└── workspace/            # agent's working directory
```

Agent only knows `ZLAW_AGENT_HOME`, not `ZLAW_HOME` (ctl's home).

## Configuration (agent.toml)

See [docs/users/configuration.md](../users/configuration.md) for full reference.

Key sections:
- `[agent]` — ID, name, description, roles
- `[llm]` — backend, model, secret reference, context_budget
- `[tools]` — allowed list, max_result_bytes
- `[adapter]` — adapter instances (telegram, fizzy, etc)
- `[sticky]` — system prompt injection rules
- `[memory]` — memory backend configuration

Secret references use env var names:

```toml
[llm]
backend = "minimax"
model = "minimax-2.7"
secret = { api_key = "$MINIMAX_API_KEY" }

[[adapter]]
type = "telegram"
secret = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

No values in config — only `$VAR_NAME` references. Values injected by ctl at spawn.

## Context Building

When a message is received:

```
System Prompt =
    SOUL.md
  + IDENTITY.md
  + Sticky blocks (self-identity, allowed-tools)
  + Tool definitions
  + Memory recall (semantic search if enabled)
  + Active skills (on-demand)

History Window =
    Last N turns (token-limited via pruning)

User Input =
    Prefill (cwd, datetime, file:...) + user message

→ LLM call
```

See [agent_contexts.md](./agent_contexts.md) for details on context engineering.

## Tool System

Tools are discovered from:
1. Built-in tools (read, write, bash, glob, grep, http_request, etc)
2. Skill files in `skills/` directory (markdown-based)

See [agent_tools.md](./agent_tools.md) and [agent_skills.md](./agent_skills.md).

## Modes

| Mode | Description |
|------|-------------|
| `run` | Single input → response → exit |
| `serve` | REPL loop, listens for input (stdin, Telegram, etc) |
| `attach` | Attach to running agent via Unix socket |

## Environment Variables

| Var | Source | Purpose |
|-----|--------|---------|
| `ZLAW_AGENT_HOME` | ctl injects at spawn | Root for all agent files |
| `ZLAW_AGENT_ID` | ctl injects | Agent ID |
| `ZLAW_NATS_URL` | ctl injects | NATS connection (standalone: not set) |
| `MINIMAX_API_KEY` | ctl injects | From secrets.toml (via env_vars mapping) |
| `ANTHROPIC_API_KEY` | ctl injects | From secrets.toml |
| `TELEGRAM_BOT_TOKEN` | ctl injects | From secrets.toml |

Agent receives secrets as env vars at spawn — no file path exposed.

Agent does NOT know about `ZLAW_HOME` or `secrets.toml`.

## See Also

- [agent_credentials.md](./agent_credentials.md) — secrets design
- [agent_contexts.md](./agent_contexts.md) — context engineering details
- [agent_tools.md](./agent_tools.md) — built-in tools reference
- [agent_skills.md](./agent_skills.md) — markdown-based skills