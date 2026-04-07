# Planning: zlaw

## Implementation Phases

Development is split into two major phases. Phase 1 (standalone agent) must be stable before Phase 2 begins.

---

## Phase 1: Standalone Agent (Start Here)

Goal: a single zlaw-agent binary that accepts input, runs an agentic loop, and emits a response. No zlaw-hub, no NATS, no inter-agent comms.

### P0 — Core Loop (Ship First)

- [ ] **Agent process bootstrap** — load config, SOUL.md, IDENTITY.md, tools manifest, restore history
- [ ] **LLM client abstraction** — interface supporting multiple backends (Anthropic, OpenAI-compat, Ollama); configured per agent
- [ ] **ReAct agentic loop** — think → tool call or respond → append to history → repeat
- [ ] **Response parser** — detect plain text vs tool call(s); support parallel tool calls
- [ ] **Conversation history manager** — append turns, track token count, trigger summarization near limit
- [ ] **Session model** — `map[sessionID → history]` from day one, even if only one session is active
- [ ] **Output emitter** — decoupled from input; same agent responds to stdin now, NATS later
- [ ] **Input handler (stdin / local HTTP)** — minimal interface for testing without Telegram

### P1 — Personality & Config

- [ ] **SOUL.md loading** — injected as system prompt personality block at bootstrap
- [ ] **IDENTITY.md loading** — agent name, role, capabilities injected into system prompt
- [ ] **agent.toml config** — model, context limits, tool ACL, isolation level, session settings
- [ ] **Hot-reload on file change** — inotify-watch config + SOUL/IDENTITY; apply without restart
- [ ] **Secret injection** — env-var based; no plaintext secrets in config files

### P2 — Tool / Skill System

- [ ] **Tool registry** — agent declares available tools; schemas included in LLM context
- [ ] **Plugin binary contract** — versioned gRPC or net/rpc interface that skill binaries implement
- [ ] **Tool executor** — spawn plugin, call over IPC, enforce timeout, return result or error
- [ ] **Tool ACL** — per-agent whitelist; executor enforces before dispatch
- [ ] **Plugin hot-reload** — reload skill binaries without restarting agent

### P3 — Memory

- [ ] **Working memory** — in-memory scratch state per session, cleared on session end
- [ ] **Long-term memory (disk)** — markdown files or simple key-value store; recalled on context build
- [ ] **Context summarization** — auto-summarize history when approaching token limit

### P4 — Observability (Phase 1)

- [ ] **Structured logging** — JSON logs with session ID and trace ID per agent
- [ ] **Token usage tracking** — log token counts per LLM call
- [ ] **Dry-run / sandbox mode** — tools no-op, LLM calls real; for testing agent behavior

---

## Phase 2: zlaw + Inter-Agent Communication

Goal: zlaw-hub process with embedded NATS, agent registry, identity verification, audit log, and pluggable interfaces.

### P0 — zlaw Core

- [ ] **zlaw-hub binary** — single process, starts embedded NATS server by default (`--embed-nats`)
- [ ] **External NATS support** — `--nats-url` flag to connect to existing NATS instance
- [ ] **Agent registry** — tracks connected agents, capabilities, health status
- [ ] **Agent connect/disconnect lifecycle** — registration on connect, heartbeat, deregister on disconnect
- [ ] **NATS subject namespace** — `agent.<n>.inbox`, `agent.<n>.outbox`, `zlaw.audit`, `zlaw.registry`

### P1 — Identity & Security

- [ ] **Agent keypairs (NKeys)** — each agent has a keypair; zlaw verifies identity on connect
- [ ] **Message signing** — A2A messages signed by sender; zlaw verifies before routing
- [ ] **Tool ACL enforcement at zlaw** — zlaw double-checks agent tool permissions on cross-agent delegations
- [ ] **Audit logger** — append-only structured log of all messages, tool calls, delegations, with trace IDs spanning agent hops

### P2 — Inter-Agent Communication

- [ ] **A2A message envelope** — structured task delegation format (from, to, task, context, reply_to, session_id)
- [ ] **Async task + reply inbox** — fire-and-forget with NATS reply subject; agents don't block waiting for peers
- [ ] **Capability advertisement** — agents publish skills manifest to registry on connect; zlaw uses for routing
- [ ] **Planner agent pattern** — designate one agent to receive user input first and delegate to peers

### P3 — Interfaces

- [ ] **Interface adapter contract** — common Go interface for input/output adapters
- [ ] **Telegram adapter** — primary interface; messages route to planner agent via zlaw
- [ ] **CLI adapter** — for local testing and scripting
- [ ] **HTTP adapter** — REST/webhook interface; enables future integrations

### P4 — Execution Isolation

- [ ] **Homedir isolation** — agent restricted to virtual home directory
- [ ] **OS user isolation** — agent spawned as dedicated OS user via sudo drop
- [ ] **Docker container isolation** — agent runs in container; connects to zlaw NATS via TCP address
- [ ] **Isolation level config** — `isolation` field in `agent.toml`; zlaw enforces at spawn time

### P5 — Observability (Phase 2)

- [ ] **Distributed trace IDs** — trace ID spans across agent hops and tool calls
- [ ] **Metrics endpoint** — Prometheus scrape target; token usage, latency, tool call counts per agent
- [ ] **Conversation replay** — replay session from audit log for debugging
- [ ] **Agent status dashboard (optional)** — read-only web UI showing agent status, task queue, recent log

---

## Nice-to-Have (Post Phase 2)

- [ ] **Scheduled task triggers** — agents register cron-style triggers (morning briefing, EOD wrap)
- [ ] **Human-in-the-loop confirmation** — agent pauses before high-risk tool execution; configurable per tool
- [ ] **Agent scaffolding CLI** — `zlaw new agent <name>` generates config, SOUL.md, IDENTITY.md, plugin stub
- [ ] **Local dev mode** — all agents as goroutines in one process, no IPC; for rapid iteration
- [ ] **Multi-tenancy** — multiple users with isolated agent contexts and session namespaces

---

## Key Design Decisions (Locked)

| Decision | Choice | Rationale |
|---|---|---|
| Language | Go | Performance, concurrency, single binary distribution |
| Message bus | NATS (embedded in zlaw) | Pub/sub native, works across Docker/OS users, already familiar |
| zlaw role | Broker (not orchestrator) | Simpler zlaw, autonomous agents, planning lives in planner agent |
| A2A routing | Always via zlaw | Centralized audit, identity verification at one point |
| Plugin system | Binary plugins over gRPC/IPC | Isolation, language agnostic, versioned contract |
| Config format | TOML | Human-friendly, Go ecosystem standard |
| Secrets | Env-var injection | No plaintext in config, works with any secrets manager |
| Personality | SOUL.md + IDENTITY.md per agent | Hot-reloadable, version-controllable, human-readable |
| Session model | `map[sessionID → history]` | Supports multi-session from day one even in single-user mode |
| Isolation levels | none → homedir → OS user → Docker | Gradual, configurable per agent |

---

## Suggested Directory Layout

```
zlaw/
├── cmd/
│   ├── zlaw-hub/     # zlaw-hub entrypoint
│   └── zlaw-agent/   # zlaw-agent binary entrypoint
├── internal/
│   ├── agent/        # Agentic loop, history, context builder
│   ├── llm/          # LLM client abstraction + backends
│   ├── tools/        # Tool executor, registry, plugin IPC
│   ├── zlaw/          # zlaw core: registry, router, audit log
│   ├── nats/         # Embedded NATS setup + subject conventions
│   ├── identity/     # Keypair management, NKeys, message signing
│   ├── adapters/     # Interface adapters (Telegram, CLI, HTTP)
│   └── config/       # Config loading, hot-reload, secret injection
├── agents/
│   └── <agent-name>/  # Per-agent: agent.toml, SOUL.md, IDENTITY.md
├── plugins/          # Skill plugin binaries and contracts
├── zlaw.toml          # Global zlaw config
└── README.md
```
