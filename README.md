# zlaw

[![Go](https://github.com/chickenzord/zlaw/actions/workflows/go.yml/badge.svg)](https://github.com/chickenzord/zlaw/actions/workflows/go.yml)
[![Lint](https://github.com/chickenzord/zlaw/actions/workflows/lint.yml/badge.svg)](https://github.com/chickenzord/zlaw/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/chickenzord/zlaw/branch/main/graph/badge.svg)](https://codecov.io/gh/chickenzord/zlaw)
[![Go version](https://img.shields.io/github/go-mod/go-version/chickenzord/zlaw)](go.mod)

> **Work in progress.** This project is under active development and not ready for everyday use. Expect breaking changes, missing features, and rough edges.

Your personal AI assistant — runs on your machine, works with any LLM, and gets more useful the longer you use it.

---

## Why zlaw

zlaw is a multi-agent platform built around a hub model: `zlaw hub start` supervises a fleet of agents over an embedded NATS bus, each with its own personality, model, and toolset. A manager agent routes tasks to specialists automatically.

Each agent runs as a separate process. Planned: configurable isolation per agent — shared user, dedicated OS user, or Docker container.

Everything runs on your machine. Config, memory, and conversation history are plain files — readable, editable, and git-trackable. Single Go binary, no extra runtime required.

---

## What you can do with it

**Have an assistant that actually knows you.**

zlaw builds a memory of facts, preferences, and context as you talk. With proactive saving on, it decides what's worth keeping on its own. You can also be explicit:

```
you: remember that we deploy every Tuesday and I prefer squash merges
```

Later, ask anything and it finds the right memory by meaning — not just keyword matching.

**Set reminders and recurring tasks by just asking.**

```
you: remind me every Monday morning to review the backlog
you: check my inbox daily at 9am and summarize anything urgent
```

The agent creates and manages its own scheduled jobs. You can also list or cancel them the same way:

```
you: what recurring tasks do you have set up?
you: cancel the inbox check
```

For bulk setup or version-controlled schedules, `cron.toml` is also supported.

**Talk to it from your phone via Telegram.**

Connect a Telegram bot and your assistant is always a message away. Each conversation thread keeps its own context — separate sessions for work, personal, and side projects.

**Run a fleet of specialist agents.**

Start `zlaw hub start` and it supervises a group of agents, each with its own personality, model, and toolset. Your manager agent takes the request and delegates automatically — the code agent handles the diff, the research agent handles the search, the calendar agent books the meeting. All on your machine, no extra infrastructure.

```
you: research the top open-source vector databases, then draft a comparison doc
```

The manager figures out who does what. You just get the result.

**Manage your session without touching the agent.**

Slash commands are handled directly — no LLM call, no cost:

```
/clear      — start a fresh conversation
/history    — review what's been said this session
/help       — list all available commands
```

Personality and behaviour files hot-reload on save — no restart needed for those either.

---

## Features

### Personal assistant
- **Any LLM** — Anthropic, or any OpenAI-compatible endpoint (Minimax, OpenRouter, Ollama, self-hosted)
- **Long-term memory** — saved as plain Markdown; recalled by semantic search; human-readable and git-trackable
- **Proactive memory saving** — agent decides what's worth keeping without being asked; or tell it explicitly
- **Scheduled tasks** — create, list, and cancel cron jobs by talking to the agent; or define them in `cron.toml`
- **Telegram** — session-aware; independent threads per conversation
- **Slash commands** — `/clear`, `/history`, `/help` handled client-side with no LLM call
- **Streaming** — tokens arrive as they're generated
- **Session persistence** — conversations stored as JSONL; resume any session by ID

### Multi-agent
- **Hub supervisor** — `zlaw hub start` spawns, monitors, and auto-restarts a fleet of agent processes
- **Agent delegation** — manager agent routes tasks to specialist peers and returns a structured result
- **Per-agent access control** — each agent gets a scoped token at spawn time; permissions enforced at the broker

### Under the hood
- **Embedded NATS** — agent-to-agent messaging over a local message bus; no external broker needed
- **Context window management** — token budget with automatic summarisation and layered pruning (thinking → tool results → turns)
- **Prompt caching** — Anthropic backends cache stable system prompt layers to cut latency and cost
- **Hot-reload** — personality, scheduled jobs, and runtime config update live on file save

---

## Quick start

```sh
# Bootstrap workspace (creates zlaw.toml, credentials.toml, manager agent)
zlaw init

# Add LLM credentials
zlaw auth login --profile anthropic --type apikey

# Run a single agent interactively
zlaw agent run -a myagent

# Run as a background daemon (enables Telegram + scheduled tasks)
zlaw agent serve -a myagent
```

To create an additional named agent:

```sh
zlaw init -a myagent
```

### Run multiple agents with a hub

```sh
zlaw hub start
```

```toml
# zlaw.toml
[[agents]]
name    = "manager"
manager = true   # receives user input, delegates to peers

[[agents]]
name = "coder"

[[agents]]
name = "researcher"
```

Each agent gets its own `agent.toml`, `SOUL.md`, and `IDENTITY.md` under `agents/<name>/`.

---

## Configuration

### Minimal `agent.toml`

```toml
[agent]
name = "myagent"

[llm]
backend      = "openrouter"
model        = "openai/gpt-4o"
auth_profile = "openrouter"
max_tokens   = 4096
```

### Memory

```toml
[sticky]
proactive_memory_save = true   # agent saves facts without being asked

[memory.embedder]
backend = "openrouter"
model   = "openai/text-embedding-3-small"
```

Memory files live under `$ZLAW_HOME/memories/<agent>/` as plain Markdown — readable and editable. The vector index is a local cache; delete it to rebuild.

### Context management

```toml
[llm]
context_token_budget         = 80000
context_summarize_threshold  = 0.8
context_summarize_turns      = 10
context_summarize_model      = "openai/gpt-4o-mini"
context_prune_levels         = ["strip_thinking", "strip_tool_results", "drop_pairs"]
max_memory_tokens            = 2000
```

---

## Docs

- [Configuration reference](docs/configuration.md) — full `agent.toml` reference, backends, credentials, context tuning
- [Built-in tools](docs/tools.md) — tool reference, allowlist, result limits
