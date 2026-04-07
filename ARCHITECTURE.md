# Architecture: zlaw

## Overview

A multi-agent personal assistant platform written in Go. The system consists of a central **zlaw** process and one or more **Agent** processes communicating over an embedded NATS message bus.

This document describes the full target architecture. Implementation starts with the standalone agent (Phase 1) before introducing zlaw and inter-agent communication.

---

## Design Principles

- **zlaw is a broker, not an orchestrator** — it routes messages, verifies identity, and logs. It does not plan or decide.
- **Agents are autonomous** — each agent runs its own agentic loop independently.
- **One planner agent** — a designated agent receives user input first and delegates to peers. Planning logic lives in an agent, not zlaw.
- **Secure by design** — agents verify each other's identity via keypairs. Prompt injection across agent boundaries is mitigated at the transport layer.
- **Simple ops** — one zlaw-hub binary, agents as separate binaries. NATS is embedded in zlaw (optional flag to use external NATS).
- **Pluggable everywhere** — interfaces (Telegram, CLI, HTTP), LLM backends, and tool plugins are all swappable via config or binary plugins over IPC.

---

## System Topology

```
[Telegram / CLI / HTTP]
          │
         zlaw-hub  ◄── single binary
          ├── Embedded NATS server (--embed-nats, default: true)
          ├── Agent registry
          ├── Identity verifier (NKeys / token per agent)
          ├── Audit logger (append-only)
          └── Interface adapters (pluggable)
                    │
              NATS message bus
         ┌──────────┼──────────┐
      agent-a     agent-b    agent-c
   (planner)   (tool)      (tool)
   own process / OS user / Docker container
   connect to zlaw's NATS via TCP
```

---

## Phase 1: Standalone Agent

> Implement this before zlaw or inter-agent communication.

A standalone agent process that accepts input, runs the agentic loop, and emits a response. No zlaw dependency.

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
│   └── Accepts message (stdin / HTTP / eventually NATS)
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

Conversation history is a `map[sessionID → history]`. Even in single-user personal assistant mode, this design is used from the start to avoid a painful refactor when zlaw introduces session routing.

---

## Phase 2: zlaw + Inter-Agent Communication

> Add after Phase 1 is stable.

### zlaw Responsibilities

| Responsibility | Notes |
|---|---|
| Embedded NATS server | `--embed-nats` flag (default true); supports external NATS via `--nats-url` |
| Agent registry | Tracks connected agents, their capabilities, and health |
| Identity verification | Each agent has a keypair (NKeys); zlaw verifies on connect |
| Audit logger | Append-only structured log of all messages, tool calls, delegations |
| Interface adapters | Telegram, CLI, HTTP — pluggable, route to planner agent |

### NATS Subject Namespace

```
agent.<name>.inbox       ← messages to a specific agent
agent.<name>.outbox      ← messages from a specific agent
zlaw.audit                ← zlaw subscribes, logs everything
zlaw.registry             ← agent registration/heartbeat
```

### Agent-to-Agent Communication

- All A2A messages route **via zlaw** (centralized, auditable, identity-verified)
- Task delegation uses a structured envelope — not raw text:

```json
{
  "from": "agent-a",
  "to": "agent-b",
  "task": "fetch_weather",
  "context": { "location": "..." },
  "reply_to": "agent.agent-a.inbox",
  "session_id": "abc123"
}
```

- Async by default: fire-and-forget with a reply inbox
- Agents advertise capabilities in registry on connect

---

## Execution Isolation Levels

Configurable per agent in `agent.toml`. From lowest to highest isolation:

| Level | Description |
|---|---|
| `none` | Same user as zlaw, shared filesystem |
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

- Per-agent: `agent.toml` — model, personality files, tools, isolation level, session config
- Global (zlaw): `zlaw.toml` — NATS settings, interface adapters, audit log path
- Hot-reload: inotify-watched; changes applied without restart
- Secrets: injected via environment variables or age-encrypted file at startup — never plaintext in config

---

## Security Model

- **Agent identity**: Each agent has a keypair. zlaw verifies on connect. Messages are signed.
- **Tool ACL**: Per-agent whitelist of allowed tools, enforced by zlaw (not self-enforced by agent).
- **Audit log**: Append-only. Every tool call, A2A message, and user interaction is logged with trace ID.
- **Prompt injection mitigation**: Cross-agent messages are verified at transport layer before reaching LLM context.
- **No ambient authority**: Agents cannot call tools not in their ACL, cannot impersonate other agents.
