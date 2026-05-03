# Agent: Standalone Mode

## Overview

A standalone agent is a self-contained process that runs the agentic loop independently. It receives input, executes tools, and produces output. No hub dependency.

## Startup Sequence

```
1. Load config via -c flag or ZLAW_AGENT_CONFIG env var  → config (model, backend)
2. Load SOUL.md (from config path or agent home)         → system prompt component (personality)
3. Load IDENTITY.md (from config path or agent home)     → system prompt component (role definition)
4. Restore history       → sessions/<id>/ (JSONL files)
5. Register tools        → from tools/ dir and skills/
6. Connect to LLM        → via configured backend (Minimax, OpenRouter, Anthropic)
7. Enter loop            → read input → build context → call LLM → execute tools
```

## Filesystem

```
$ZLAW_HOME/
├── agent-assistant.toml    # ctl-owned agent configuration
├── agent-dev.toml
└── agents/                 # agent runtime data
    ├── assistant/
    │   ├── SOUL.md         # personality (hot-reload on change)
    │   ├── IDENTITY.md     # role definition (hot-reload on change)
    │   ├── skills/         # per-agent skill files
    │   ├── sessions/      # conversation history
    │   │   └── <session-id>.jsonl
    │   ├── memories/      # long-term memory
    │   │   ├── <topic>.md
    │   │   └── vector.db
    │   └── workspace/      # agent's working directory
    └── dev/
        └── ...

$ZLAW_AGENT_HOME/           # set by ZLAW_AGENT_HOME env var (points to agents/{id}/)
├── runtime.toml            # runtime overrides
├── SOUL.md
├── IDENTITY.md
├── skills/
├── sessions/
├── memories/
└── workspace/
```

**Config vs Runtime:**
- Agent config (`llm`, `adapter`, etc.) is owned by ctl at `$ZLAW_HOME/agent-{id}.toml`
- Agent runtime data (SOUL.md, IDENTITY.md, sessions, memories, workspace) is in `$ZLAW_HOME/agents/{id}/`

Agent receives config file path via `-c` flag or `ZLAW_AGENT_CONFIG` env var at spawn. Agent only knows `ZLAW_AGENT_HOME`, not `ZLAW_HOME` parent.

## Configuration (agent-{id}.toml)

See [docs/users/configuration.md](../users/configuration.md) for full reference.

Key sections:
- `[llm]` — backend, model, context_budget
- `[tools]` — allowed list, max_result_bytes
- `[adapter]` — adapter instances (telegram, fizzy, etc)
- `[sticky]` — system prompt injection rules
- `[memory]` — memory backend configuration

Note: Secret references use env var names (`$VAR_NAME`). Values injected by ctl at spawn.

```toml
[llm]
backend = "anthropic"
client_config = {
  base_url = "https://api.anthropic.com",
  api_key = "$ANTHROPIC_API_KEY"
}
model = "claude-sonnet-4"
model_config = {
  max_tokens = 8192,
  timeout_sec = 120
}

[[adapter]]
backend = "telegram"
client_config = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

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
| `ZLAW_AGENT_HOME` | ctl injects at spawn | Root for agent runtime files |
| `ZLAW_AGENT_ID` | ctl injects | Agent ID |
| `ZLAW_AGENT_CONFIG` | ctl injects | Path to agent config file |
| `ZLAW_NATS_URL` | ctl injects | NATS connection (standalone: not set) |
| `MINIMAX_API_KEY` | ctl injects | From secrets.toml (via env_vars mapping) |
| `ANTHROPIC_API_KEY` | ctl injects | From secrets.toml |
| `TELEGRAM_BOT_TOKEN` | ctl injects | From secrets.toml |

Agent receives secrets as env vars at spawn — no file path exposed.

Agent does NOT know about `ZLAW_HOME` or `secrets.toml`.

## See Also

- [agent_secrets.md](./agent_secrets.md) — secrets design
- [agent_contexts.md](./agent_contexts.md) — context engineering details
- [agent_tools.md](./agent_tools.md) — built-in tools reference
- [agent_skills.md](./agent_skills.md) — markdown-based skills
- [Agent Config Ownership Refactoring](../plans/refactor/06-agent-config-ownership.md)