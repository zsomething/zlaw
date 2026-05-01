# Agent: Standalone Mode

## Overview

A standalone agent is a self-contained process that runs the agentic loop independently. It receives input, executes tools, and produces output. No hub dependency.

## Startup Sequence

```
1. Load agent.toml       → config (model, backend, auth profile, tools)
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
├── agent.toml             # configuration
├── runtime.toml           # runtime overrides (watched, hot-reloaded)
├── credentials.toml       # written by hub at spawn time
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

## Configuration (agent.toml)

See [docs/users/configuration.md](../users/configuration.md) for full reference.

Key sections:
- `[agent]` — ID, name, description, roles
- `[llm]` — backend, model, auth_profile, context_budget
- `[tools]` — allowed list, max_result_bytes
- `[adapter]` — adapter instances (telegram, fizzy, etc)
- `[sticky]` — system prompt injection rules
- `[memory]` — memory backend configuration

## Context Building

When a message is received:

```
System Prompt =
    SOUL.md
  + IDENTITY.md
  + Sticky blocks (self-identity, allowed-tools)
  + Tool definitions
  + Memory recall (semantic search if enabled)

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
| `ZLAW_AGENT_HOME` | Hub injects at spawn | Root for all agent files |
| `ZLAW_NATS_URL` | Hub injects | NATS connection (standalone: not set) |
| `ZLAW_CREDENTIALS_FILE` | Hub injects | Path to credentials profile |
| `ZLAW_AGENT` | Hub injects | Agent ID |
| `ZLAW_HOME` | ctl sets | Hub's home (agent should not use) |

## See Also

- [agent_contexts.md](./agent_contexts.md) — context engineering details
- [tools.md](./tools.md) — built-in tools reference
- [skills.md](./skills.md) — markdown-based skills