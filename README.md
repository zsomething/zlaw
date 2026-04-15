# zlaw

[![Go](https://github.com/zsomething/zlaw/actions/workflows/go.yml/badge.svg)](https://github.com/zsomething/zlaw/actions/workflows/go.yml)
[![Lint](https://github.com/zsomething/zlaw/actions/workflows/lint.yml/badge.svg)](https://github.com/zsomething/zlaw/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/zsomething/zlaw/branch/main/graph/badge.svg)](https://codecov.io/gh/zsomething/zlaw)
[![Go version](https://img.shields.io/github/go-mod/go-version/zsomething/zlaw)](go.mod)

> **Work in progress.** This project is under active development and not ready for everyday use. Expect breaking changes, missing features, and rough edges.

Your personal AI assistant — runs on your machine, works with any LLM, and gets more useful the longer you use it.

---

## Goals

**A platform for autonomous agents that work for you.**
zlaw is built around the idea that a fleet of specialized agents — each with its own personality, model, and toolset — can handle complex, multi-step tasks better than a single monolithic assistant.

**Agents that remember and grow.**
Long-term memory stored as plain files, searchable by meaning. The agent builds context over time without manual curation.

**You own your data.**
Everything — config, memory, history — is a plain file you can read, edit, and version-control. No proprietary storage.

**Any model, any endpoint.**
Works with Anthropic or any OpenAI-compatible API. Swap models without changing behaviour.

---

## Non-Goals

**Minimal resource footprint.**
Go is chosen for performance and developer experience, not to run on a Raspberry Pi. If you need a lightweight agent, look elsewhere.

**General-purpose AI infrastructure.**
zlaw is a personal assistant platform, not a replacement for enterprise AI platforms like LangSmith or Weights & Biases.

**Agent-to-agent direct communication.**
All inter-agent communication routes through the hub (NATS broker). No peer-to-peer networking.

**Plugin ecosystem for external services.**
Skills are Markdown files executed locally. gRPC/plugin binaries are for local skill tooling only — not for integrating with external SaaS.

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

**Adapters are optional.**

Each agent can have zero or more adapters — CLI, Telegram, or others. A common pattern is a headless specialist fleet (no adapters) that only responds to delegation from the manager. But you're also free to give agents their own Telegram bots for direct human interaction, or mix and match as needed.

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

**Hub supervisor**
- `zlaw hub start` spawns, monitors, and auto-restarts a fleet of agent processes
- Configurable restart policy per agent: always, on-failure, or never
- Per-agent credential injection at spawn time — no secrets in config files

**Agent registry & discovery**
- Agents register on connect and send heartbeats every 30s
- Hub maintains a live registry of connected agents with their capabilities and roles
- All agents can query the registry to discover peers for delegation

**A2A delegation**
- Manager agent routes tasks to specialist peers via the `agent_delegate` tool
- Tasks are wrapped in a structured envelope with result schema
- Messages are durable — JetStream persists them until acknowledged
- Manager agents can stop or restart peer agents via `agent_stop` / `agent_restart`

**Security model**
- Each agent gets a scoped NATS token at spawn time
- Permissions enforced at the broker: specialists can only publish to manager inbox and registry; managers can publish to any agent inbox
- No agent can create or remove other agents — those operations are hub CLI-only
- Lifecycle tools include self-protection: manager cannot stop/restart itself

**Durable messaging**
- All inter-agent messages flow through JetStream streams
- Unacked messages are redelivered on reconnect — no lost tasks
- WorkQueue retention: messages are deleted after successful processing

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
