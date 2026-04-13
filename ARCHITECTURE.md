# Architecture: zlaw

## Overview

A multi-agent personal assistant platform written in Go. The system consists of a central **zlaw-hub** process and one or more **Agent** processes communicating over an embedded NATS message bus.

This document describes the full target architecture. Implementation starts with the standalone agent (Phase 1) before introducing zlaw-hub and inter-agent communication.

---

## Design Principles

- **Hub is a broker and process manager, not a task orchestrator** — it routes messages, verifies identity, supervises agent processes, and logs. It does not plan or decide.
- **Agents are autonomous** — each agent runs its own agentic loop independently.
- **One manager agent** — a designated agent receives user input first and delegates to peers. Task routing logic lives in the manager agent, not the hub.
- **Adapters live in agents** — Telegram, CLI, and other interfaces are owned by the agent process (typically the manager). Hub does not own adapters.
- **Secure by design** — agents verify each other's identity via keypairs. Prompt injection across agent boundaries is mitigated at the transport layer.
- **Simple ops** — one zlaw-hub binary, agents as separate binaries. NATS is embedded in zlaw-hub by default.
- **Pluggable everywhere** — LLM backends and tool plugins are all swappable via config or binary plugins over IPC.

---

## System Topology

```
[Telegram / CLI / HTTP]
         │
    manager-agent   ◄── owns adapters; receives all user input
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
         │
    ┌────┴─────────────┐
  agent-a           agent-b
 (specialist)     (specialist)
 own process     own process
 connect to hub's NATS via TCP / Unix socket
```

---

## Phase 1: Standalone Agent

> Implement this before zlaw-hub or inter-agent communication.

A standalone agent process that accepts input, runs the agentic loop, and emits a response. No zlaw-hub dependency.

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

Conversation history is a `map[sessionID → history]`. Even in single-user personal assistant mode, this design is used from the start to avoid a painful refactor when zlaw-hub introduces session routing.

---

## Phase 2: zlaw-hub + Inter-Agent Communication

> Add after Phase 1 is stable.

### Hub Responsibilities

| Responsibility | Notes |
|---|---|
| Embedded NATS server | `--embed-nats` flag (default true); supports external NATS via `--nats-url` |
| Agent supervisor | Spawns, stops, and restarts agent processes; configurable restart policy per agent |
| Agent registry | Tracks connected agents, their capabilities, and health |
| NATS ACL enforcement | Per-agent publish/subscribe permissions; enforced at broker level |
| Identity verification | Each agent has a keypair (NKeys); hub verifies on connect |
| Audit logger | Append-only structured log of all messages, tool calls, and delegations |

Hub does **not** own interface adapters. Agents (typically the manager) own their own Telegram, CLI, or HTTP interfaces.

### Manager Agent

The manager agent is a **regular agent** with two differences:

1. **Extra tool set** — hub-management tools available only to agents with `manager: true` in `agent.toml` (enforced by hub ACL).
2. **Self-protection** — `agent_remove` refuses to target itself; hub enforces this.

Everything else is identical: it has a personality, builds long-term memory, can be chatted with directly, runs its own agentic loop, and owns its Telegram/CLI adapters. It evolves like any other agent.

| Hub-management tool | What it does |
|---|---|
| `agent_create(name, role, description)` | Scaffold agent dir + files, register with hub, spawn process |
| `agent_list()` | All registered agents and their status |
| `agent_configure(name, key, value)` | Write to agent's `runtime.toml`, triggers hot-reload |
| `agent_stop(name)` / `agent_restart(name)` | Process lifecycle control |
| `agent_delegate(name, task, context)` | Publish task envelope to `agent.<name>.inbox` via NATS |

### Agent-to-Agent Communication

The `agent_delegate` tool is a thin wrapper around a NATS publish. The tool constructs a structured envelope and publishes to `agent.<name>.inbox`. Hub's NATS ACL layer enforces which agents can publish to which subjects. No hub business logic is in the path — enforcement is at the broker transport layer.

Middleware that hub applies (composable):
- **ACL** — verify source agent has publish permission for the target subject
- **Audit** — hub subscribes to all subjects; every message lands in the audit log
- **Rate limiting** — optional, per-agent publish rate (future)
- **Signing verification** — reject unsigned messages (Phase 2 identity)

Message envelope:
```json
{
  "from": "manager",
  "to": "agent-b",
  "task": "fetch_weather",
  "context": { "location": "..." },
  "reply_to": "agent.manager.inbox",
  "session_id": "abc123",
  "trace_id": "xyz789"
}
```

Async by default: fire-and-forget with a reply inbox. Agents advertise capabilities in the registry on connect.

### NATS Subject Namespace

```
agent.<name>.inbox       ← inbound tasks/messages for a specific agent
agent.<name>.outbox      ← responses/events from a specific agent
zlaw.hub.inbox           ← hub management requests (agent_create etc. reach hub here)
zlaw.audit               ← hub subscribes; logs everything
zlaw.registry            ← agent registration/heartbeat
```

### Default ACL by Agent Type

| Agent type | Can publish to | Can subscribe to |
|---|---|---|
| Manager | `agent.*.inbox`, `zlaw.hub.inbox` | `agent.manager.inbox`, `zlaw.registry` |
| Specialist | `agent.manager.inbox` (reply only) | `agent.<own-name>.inbox` |
| Hub | `zlaw.audit`, `zlaw.registry` | all |

---

## Configuration Boundaries

Clear ownership of every config file:

| File | Owned by | Read by | Contents |
|------|----------|---------|----------|
| `zlaw.toml` | Hub | Hub only | NATS settings, agent registry (name → dir), hub keypair path, audit log path |
| `credentials.toml` | Hub | Hub only at spawn time | LLM API keys, Telegram bot token, OAuth2 profiles — injected into agents as env vars |
| `agents/<name>/agent.toml` | Agent | Hub (to spawn) + Agent (to run) | LLM backend, auth profile ref, tool ACL, context budget, isolation level, `manager: true/false` |
| `agents/<name>/runtime.toml` | Agent (writes) | Agent + Hub watches | Dynamic overrides: model switching, flag toggles |
| `agents/<name>/cron.toml` | Agent | Agent only | Scheduled tasks |
| `agents/<name>/SOUL.md` | Agent | Agent only | Personality |
| `agents/<name>/IDENTITY.md` | Agent | Agent only | Role definition |
| `sessions/<name>/` | Agent | Agent only | Conversation JSONL history |
| `memories/<name>/` | Agent | Agent only | Long-term memory Markdown files + vector index |

**Key invariant: credentials never flow directly to agents.** Hub reads `credentials.toml` and injects referenced profiles as environment variables at agent spawn time. Agents never read `credentials.toml`.

---

## User Journey

### Day 0 — First install

```
zlaw-hub init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton with one manager agent entry)
#            $ZLAW_HOME/credentials.toml (empty template, 0600)
#            $ZLAW_HOME/agents/manager/ (full agent scaffold)

zlaw-hub auth add --provider anthropic --name default
# Prompts for API key → saves to credentials.toml

zlaw-hub auth add --provider telegram --name default
# Prompts for bot token → saves to credentials.toml, updates zlaw.toml

zlaw-hub start
# Starts hub, spawns manager agent
# Telegram is live. User can chat immediately.
```

### Day 1 — Growing the agent team

User messages the manager via Telegram:
> "Create a coding assistant for me"

Manager calls `agent_create(name="coding", role="Go developer")`:
- Hub scaffolds `$ZLAW_HOME/agents/coding/` with agent.toml + personality files
- Hub spawns the process, registers in registry
- Manager confirms: "Done. I can delegate coding work to it now."

### Day N — Operations

```bash
zlaw-hub status                   # hub health + per-agent status
zlaw-hub agent list               # all agents, status, last heartbeat
zlaw-hub agent logs coding        # stream agent logs
zlaw-hub agent restart coding     # restart agent process
zlaw-hub agent remove coding      # stop process + deregister
```

---

## Execution Isolation Levels

Configurable per agent in `agent.toml`. From lowest to highest isolation:

| Level | Description |
|---|---|
| `none` | Same user as hub, shared filesystem |
| `homedir` | Agent restricted to its own virtual home directory |
| `user` | Agent runs as a dedicated OS user (sudo drop) |
| `container` | Agent runs inside a Docker container, connects to NATS by TCP address |

---

## Plugin / Skill System

- Tools are binary plugins implementing a versioned gRPC/IPC contract (Go interface + protobuf)
- Agent declares available tools in `tools.toml` or `tools/` directory
- Tool executor spawns plugin, calls over IPC, enforces timeout
- Plugins can be hot-reloaded without restarting the agent

---

## Configuration

- Per-agent: `agent.toml` — model, personality files, tools, isolation level, session config, `manager` flag
- Global (hub): `zlaw.toml` — NATS settings, agent registry, audit log path
- Credentials: `credentials.toml` — hub-owned, injected as env vars at spawn time
- Hot-reload: fsnotify-watched; changes applied without restart
- Secrets: injected via environment variables at spawn — never plaintext in agent config

---

## Security Model

- **Agent identity**: Each agent has a keypair (NKeys). Hub verifies on connect. Messages are signed.
- **NATS ACL**: Hub enforces per-agent publish/subscribe permissions at the broker layer. No business logic required.
- **Audit log**: Append-only. Every tool call, A2A message, and user interaction is logged with trace ID.
- **Prompt injection mitigation**: Cross-agent messages are verified at transport layer before reaching LLM context.
- **No ambient authority**: Agents cannot publish to subjects outside their ACL, cannot impersonate other agents.
- **Credential isolation**: Agents never read credentials.toml directly; hub injects only the referenced profile as env vars.
- **Manager self-protection**: Hub rejects `agent_remove` targeting the manager agent itself.
