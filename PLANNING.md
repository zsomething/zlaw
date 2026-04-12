# Planning: zlaw

## Implementation Phases

Development is split into two major phases. Phase 1 (standalone agent) must be stable before Phase 2 begins.

---

## Phase 1: Standalone Agent ✓

Goal: a single zlaw-agent binary that accepts input, runs an agentic loop, and emits a response. No zlaw-hub, no NATS, no inter-agent comms.

### P0 — Core Loop

- [x] **Agent process bootstrap** — load config, SOUL.md, IDENTITY.md, tools manifest, restore history
- [x] **LLM client abstraction** — interface supporting multiple backends (Anthropic, OpenAI-compat); configured per agent
- [x] **ReAct agentic loop** — think → tool call or respond → append to history → repeat; up to 20 iterations per turn
- [x] **Response parser** — detect plain text vs tool call(s); parallel tool calls run concurrently
- [x] **Conversation history manager** — append turns, track token count via API response, trigger summarisation near limit
- [x] **Session model** — `map[sessionID → history]` with JSONL persistence under `$ZLAW_HOME/sessions/<agent>/`
- [x] **Output emitter** — streaming tokens to terminal; decoupled from input handler
- [x] **Input handler** — interactive terminal (stdin) and one-shot stdin pipe; daemon mode over Unix socket

### P1 — Personality & Config

- [x] **SOUL.md loading** — injected as system prompt personality block at bootstrap
- [x] **IDENTITY.md loading** — agent name, role, capabilities injected into system prompt
- [x] **agent.toml config** — model, context limits, tool ACL, adapter, session settings; env-var expansion
- [x] **Hot-reload on file change** — fsnotify watch on SOUL.md, IDENTITY.md, agent.toml, cron.toml; live on next message
- [x] **Secret injection** — env-var based; no plaintext secrets in config files; credentials.toml with apikey and oauth2 profiles
- [x] **Runtime config overrides** — `runtime.toml` for hot model switching without full config reload

### P2 — Tool / Skill System

- [x] **Tool registry** — agent declares available tools; schemas included in LLM context
- [x] **Tool ACL** — per-agent allowlist; result size cap; executor enforces before dispatch
- [x] **Built-in tools** — file I/O, bash, glob, grep, web fetch/search, HTTP request, memory ops, cron management, configure
- [x] **Skills discovery** — scan `$ZLAW_HOME/skills/` for Markdown skill files; inject index into system prompt; load on demand via `skill_load` tool
- [ ] **Plugin binary contract** — versioned gRPC or net/rpc interface for external skill binaries
- [ ] **Tool executor (plugin IPC)** — spawn plugin binary, call over IPC, enforce timeout, return result or error
- [ ] **Plugin hot-reload** — reload skill binaries without restarting agent

### P3 — Memory

- [x] **Long-term memory (disk)** — Markdown files with YAML frontmatter under `$ZLAW_HOME/memories/<agent>/`; human-readable, git-trackable
- [x] **Semantic memory search** — vector index via chromem-go persisted alongside memory files; embedding backend configurable per agent; content-hash diffing avoids redundant re-embedding
- [x] **Proactive memory save** — sticky `[Memory behavior]` instruction block tells agent to save without being asked; opt-in via `sticky.proactive_memory_save`
- [x] **Memory injection** — all memories loaded into system prompt as uncached section at session start; token-capped
- [x] **Context summarisation** — auto-summarise oldest turns when approaching token budget; configurable threshold, turn count, and optional separate summarisation model
- [ ] **Working memory** — per-session scratch state separate from conversation history; cleared on session end

### P4 — Context Engineering

- [x] **Token budget and pruning** — hard token limit; multi-level pruning pipeline: strip extended thinking → strip tool result bodies → drop full turn pairs
- [x] **Prompt caching (Anthropic)** — system prompt split into stable cached layers; framework instructions, personality, and skills each get their own checkpoint
- [x] **Session prefill** — inject working directory, current time, or file contents into first user message; keeps system prompt cache clean across sessions

### P5 — Interfaces & Adapters

- [x] **CLI adapter** — interactive REPL and stdin pipe; session ID flag; verbose mode; token usage display
- [x] **Daemon mode** — `serve` command runs agent as Unix socket server; sessions managed independently per client
- [x] **Session attach** — `attach` command connects a terminal to a running daemon session
- [x] **Telegram adapter** — full bot integration; session-per-chat; message formatting; inline streaming
- [x] **Push notifications** — agent can push messages to Telegram outside of a user turn (e.g. from cron jobs)

### P6 — Scheduled Tasks

- [x] **Cron scheduler** — cron.toml defines recurring agent jobs; Go cron expression parser; jobs run as agent turns
- [x] **Cron tools** — `list_cronjobs`, `create_cronjob`, `delete_cronjob` let the agent manage its own schedule
- [x] **Hot-reload** — cron.toml changes apply without restart

### P7 — Observability

- [x] **Structured logging** — slog with `agent`, `session_id` on every line; DEBUG/INFO via `ZLAW_LOG_LEVEL`
- [x] **Token usage tracking** — cumulative input/output tokens per turn; display on request
- [ ] **Dry-run / sandbox mode** — tools no-op, LLM calls real; for testing agent behaviour

### P8 — CLI & Bootstrap

- [x] **Agent init** — `zlaw-agent init --name <n>` generates agent.toml, SOUL.md, IDENTITY.md with starter content
- [x] **Auth management** — `zlaw-agent auth login` stores credentials in credentials.toml

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

### P3 — Execution Isolation

- [ ] **Homedir isolation** — agent restricted to virtual home directory
- [ ] **OS user isolation** — agent spawned as dedicated OS user via sudo drop
- [ ] **Docker container isolation** — agent runs in container; connects to zlaw NATS via TCP
- [ ] **Isolation level config** — `isolation` field in agent.toml; zlaw enforces at spawn time

### P4 — Observability (Phase 2)

- [ ] **Distributed trace IDs** — trace ID spans across agent hops and tool calls
- [ ] **Metrics endpoint** — Prometheus scrape target; token usage, latency, tool call counts per agent
- [ ] **Conversation replay** — replay session from audit log for debugging
- [ ] **Agent status dashboard (optional)** — read-only web UI showing agent status, task queue, recent log

---

## Nice-to-Have (Post Phase 2)

- [x] ~~**Scheduled task triggers**~~ — done in Phase 1 (cron.toml + scheduler)
- [x] ~~**Agent scaffolding CLI**~~ — done in Phase 1 (`zlaw-agent init`)
- [ ] **Human-in-the-loop confirmation** — agent pauses before high-risk tool execution; configurable per tool
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
| Memory storage | Markdown files (source of truth) + vector index (cache) | Human-readable, git-trackable; index is regenerable |

---

## Suggested Directory Layout

```
zlaw/
├── cmd/
│   ├── zlaw-hub/     # zlaw-hub entrypoint
│   └── zlaw-agent/   # zlaw-agent binary entrypoint
├── internal/
│   ├── agent/        # Agentic loop, history, context builder, memory
│   ├── llm/          # LLM client abstraction + backends
│   ├── tools/        # Tool executor, registry, built-in tools
│   ├── cron/         # Cron expression parser + scheduler
│   ├── skills/       # Skill discovery and loading
│   ├── session/      # Session manager, sink, events
│   ├── adapters/     # Interface adapters (CLI, Telegram, daemon)
│   ├── transport/    # Unix socket transport
│   ├── config/       # Config loading, hot-reload, secret injection
│   ├── zlaw/         # zlaw core: registry, router, audit log (Phase 2)
│   ├── nats/         # Embedded NATS setup + subject conventions (Phase 2)
│   └── identity/     # Keypair management, NKeys, message signing (Phase 2)
├── agents/
│   └── <agent-name>/  # Per-agent: agent.toml, SOUL.md, IDENTITY.md
├── docs/              # Configuration reference, tools reference
├── plugins/           # Skill plugin binaries and contracts
├── zlaw.toml          # Global zlaw config
└── README.md
```
