# Architecture: zlaw

## Overview

Multi-agent personal assistant platform in Go. Central **zlaw-hub** + one or more **Agent** processes over embedded NATS.

Doc: full target architecture. Phase 1 = standalone agent. Phase 2 = zlaw-hub + inter-agent.

---

## Design Principles

- **Hub is broker + process manager, not task orchestrator** — routes msgs, verifies identity, supervises agents, logs. No planning.
- **Agents are autonomous** — each runs own agentic loop independently. Any agent can receive user input if it has an adapter.
- **Adapters live in agents** — Telegram, CLI, etc. owned by agent. Hub owns no adapters.
- **Peer-to-peer delegation** — agents communicate directly over NATS. Hub provides routing + ACL only.
- **Secure by design** — agents verify identity via keypairs. Prompt injection mitigated at transport layer.
- **Simple ops** — single `zlaw` binary with subcommands. NATS embedded in hub by default.
- **Pluggable everywhere** — LLM backends + tool plugins swappable via config or binary plugins over IPC.

---

## System Topology

```
[Telegram / CLI / HTTP adapters]
         │
    ┌────┴─────────────┐
  agent-a           agent-b
 (has adapter)     (headless)
 own process       own process
         │
    NATS message bus  ◄── embedded in zlaw-hub
         │
    zlaw-hub
    ├── Embedded NATS server
    ├── Agent supervisor (spawn / stop / restart)
    ├── Agent registry (capabilities, health)
    ├── NATS ACL enforcement (per-agent publish permissions)
    ├── Identity verifier (NKeys / token per agent)
    └── Audit logger (append-only, subscribes to all subjects)
```

---

## Phase 1: Standalone Agent ✓ (complete)

Standalone agent: accepts input, runs agentic loop, emits response. No zlaw-hub dependency. Implemented as `cmd/zlaw/` single binary with `run`/`serve`/`attach`/`auth`/`init` subcommands.

### Agent Process Components

```
Agent Binary
├── Bootstrap
│   ├── Load agent.toml (config)
│   ├── Load SOUL.md + IDENTITY.md → system prompt
│   ├── Load tools manifest → register skills
│   └── Restore conversation history (if any)
│
├── Input Handler
│   └── Accepts message (stdin / HTTP / Telegram / eventually NATS)
│
├── Context Builder
│   ├── System prompt (SOUL + IDENTITY + tool definitions)
│   ├── Conversation history (windowed)
│   └── Injected context (time, memory recall, agent state)
│
├── LLM Client (abstracted, swappable)
│   ├── Handles streaming, retries, token counting
│   └── Configured per agent (model, endpoint, API key)
│
├── Response Parser
│   ├── Detects: plain text vs tool call(s)
│   └── Multiple tool calls → parallel execution where safe
│
├── Tool Executor
│   ├── Dispatches to skill plugins (binary over IPC/gRPC)
│   ├── Enforces per-tool timeout
│   └── Returns result or structured error
│
├── History Manager
│   ├── Appends: user → assistant → tool calls → tool results
│   ├── Tracks token count
│   └── Triggers summarization near context limit
│
└── Output Emitter
    └── Returns final response to caller (decoupled from input)
```

### Agentic Loop (ReAct)

```
Input received
     │
     ▼
Build context (system prompt + history + tools)
     │
     ▼
LLM call
     │
     ├── Tool call requested?
     │        │
     │       YES → Execute tool(s) → Append result to history → loop back
     │        │
     │        NO
     │        │
     ▼        ▼
Emit response (end of loop)
```

### Agent State

| State | Description |
|---|---|
| `config` | Model, limits, tool ACL, isolation level |
| `identity` | Loaded from IDENTITY.md |
| `personality` | Loaded from SOUL.md |
| `tool_registry` | Available skills and their schemas |
| `conversation` | History + token count, keyed by session ID |
| `working_memory` | Scratch state within a session (in-memory) |
| `long_term_memory` | Persisted to disk, recalled on context build |

### Session Model

History = `map[sessionID → history]`. Single-user mode still uses this — avoids painful refactor when hub adds session routing.

---

## Phase 2: zlaw-hub + Inter-Agent Communication (in progress)

Hub internals partially implemented (`internal/hub/`). Hub CLI bootstrap not yet done.

### Hub Responsibilities

| Responsibility | Notes |
|---|---|
| Embedded NATS server | `--embed-nats` flag (default true); supports external NATS via `--nats-url` |
| Agent supervisor | Spawns, stops, restarts agent processes; configurable restart policy per agent |
| Agent registry | Tracks connected agents, capabilities, health |
| NATS ACL enforcement | Per-agent publish/subscribe permissions; enforced at broker level |
| Identity verification | Each agent has keypair (NKeys); hub verifies on connect |
| Audit logger | Append-only structured log of all messages, tool calls, delegations |

Hub owns no interface adapters — agents own Telegram, CLI, HTTP adapters.

### Agent Lifecycle Tools

Lifecycle tools are CLI-only (via `ctl`). All agents are equal peers.
| Tool | Notes |
|------|-------|
| `agent_create(id, role)` | CLI: scaffolds agent dir, registers with hub |
| `agent_list()` | All agents: list registered agents and status |
| `agent_configure(id, key, value)` | CLI: update agent runtime config |
| `agent_stop(id)` / `agent_restart(id)` | CLI: process lifecycle control |
| `agent_delegate(id, task, context)` | All agents: P2P delegation over NATS |

Hub middleware (composable):
- **ACL** — verify source agent has publish permission for target subject
- **Audit** — hub subscribes to all subjects; every message logged
- **Rate limiting** — optional, per-agent publish rate (future)
- **Signing verification** — reject unsigned messages (Phase 2 identity)

Message envelope:
```json
{
  "from": "alice",
  "to": "bob",
  "task": "fetch_weather",
  "context": { "location": "..." },
  "reply_to": "agent.alice.inbox",
  "session_id": "abc123",
  "trace_id": "xyz789"
}
```

Async by default: fire-and-forget with reply inbox. Agents advertise capabilities in registry on connect.

### NATS Subject Namespace

```
agent.<id>.inbox       ← inbound tasks/messages for a specific agent
agent.<id>.outbox      ← responses/events from a specific agent
zlaw.hub.inbox        ← hub management requests (ctl only via control socket)
zlaw.audit            ← hub subscribes; logs everything
zlaw.registry         ← agent registration/heartbeat
```

### Default ACL

All agents have equal P2P permissions:
- Subscribe: `zlaw.registry`, `agent.<id>.inbox` (own only), `_INBOX.>`, `$JS.API.>`
- Publish: `agent.*.inbox` (delegate to any), `zlaw.registry`, `_INBOX.>`, `$JS.API.>`

---

## Configuration Boundaries

| File | Owned by | Read by | Contents |
|------|----------|---------|----------|
| `zlaw.toml` | Hub | Hub + ctl | NATS settings, agent registry (id → dir), hub keypair path, audit log path |
| `credentials.toml` | Hub | Hub at spawn time | LLM API keys, adapter tokens — injected into agents as env vars |
| `agents/<id>/agent.toml` | Agent | Hub (to spawn) + Agent | LLM backend, auth profile ref, tool ACL, context budget, isolation level |
| `agents/<id>/runtime.toml` | Agent (writes) | Agent + Hub watches | Dynamic overrides: model switching, flag toggles |
| `agents/<id>/cron.toml` | Agent | Agent only | Scheduled tasks |
| `agents/<id>/SOUL.md` | Agent | Agent only | Personality |
| `agents/<id>/IDENTITY.md` | Agent | Agent only | Role definition |
| `sessions/<id>/` | Agent | Agent only | Conversation JSONL history |
| `memories/<id>/` | Agent | Agent only | Long-term memory Markdown files + vector index |

**Key invariant: credentials never flow directly to agents.** Hub reads `credentials.toml`, injects referenced profiles as env vars at spawn time. Agents never read `credentials.toml`.

---

## User Journey

### Day 0 — First install

```
zlaw init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton with first agent entry)
#            $ZLAW_HOME/credentials.toml (empty template, 0600)
#            $ZLAW_HOME/agents/<id> (full agent scaffold)

zlaw auth login --profile anthropic --type apikey
# Prompts for API key → saves to credentials.toml

zlaw hub start
# Starts hub, spawns registered agents
# Telegram is live. User can chat immediately.
```

### Day 1 — Growing the agent team

User runs `zlaw ctl create agent id=coding role="Go developer"`:
- ctl scaffolds `$ZLAW_HOME/agents/coding/` with agent.toml + personality files
- ctl registers with hub, hub spawns process
- User: "Done. I can delegate coding work to it now."

### Day N — Operations

```bash
zlaw ctl get agents                # list all agents, status, last heartbeat
zlaw ctl get agent <id>           # get agent details
zlaw ctl get hub                  # hub health + per-agent status
zlaw ctl logs <id> [--follow]     # stream agent logs
zlaw ctl stop <id>                # stop agent process
zlaw ctl restart <id>             # restart agent process
zlaw ctl delete <id>              # stop + deregister
zlaw ctl configure <id> <key> <value>  # update runtime config
```

---

## Execution Isolation Levels

Configurable per agent in `agent.toml`. Low to high:

| Level | Description |
|---|---|
| `none` | Same user as hub, shared filesystem |
| `homedir` | Agent restricted to own virtual home directory |
| `user` | Agent runs as dedicated OS user (sudo drop) |
| `container` | Agent runs inside Docker container, connects to NATS by TCP address |

---

## Plugin / Skill System

- Tools = binary plugins implementing versioned gRPC/IPC contract (Go interface + protobuf)
- Agent declares available tools in `tools.toml` or `tools/` directory
- Tool executor spawns plugin, calls over IPC, enforces timeout
- Plugins hot-reloadable without restarting agent

---

## Configuration

- Per-agent: `agent.toml` — model, personality files, tools, isolation level, session config
- Global (hub): `zlaw.toml` — NATS settings, agent registry, audit log path
- Credentials: `credentials.toml` — hub-owned, injected as env vars at spawn time
- Hot-reload: fsnotify-watched; changes applied without restart
- Secrets: injected via env vars at spawn — never plaintext in agent config

---

## Security Model

- **Agent identity**: Each agent has keypair (NKeys). Hub verifies on connect. Messages signed.
- **NATS ACL**: Hub enforces per-agent publish/subscribe at broker layer. No business logic required.
- **Audit log**: Append-only. Every tool call, A2A message, user interaction logged with trace ID.
- **Prompt injection mitigation**: Cross-agent messages verified at transport layer before reaching LLM context.
- **No ambient authority**: Agents cannot publish outside ACL, cannot impersonate others.
- **Credential isolation**: Agents never read `credentials.toml`; hub injects only referenced profile as env vars.
- **Self-protection**: Hub rejects lifecycle requests (stop/delete) from the target agent itself.