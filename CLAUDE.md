# CLAUDE.md — Project Context for Claude Code

This file gives Claude Code the full context needed to work on this project. Read ARCHITECTURE.md and PLANNING.md for full detail. This file summarizes the essentials and provides working conventions.

---

## What This Project Is

A multi-agent personal assistant platform written in Go. A central **zlaw** process brokers communication between autonomous **Agent** processes over an embedded NATS message bus.

Primary use case: personal assistant (Telegram as main interface). Coding assistance is a nice-to-have.

---

## Current Implementation Phase

**Phase 1: Standalone Agent**

We are building a single zlaw-agent binary that runs independently — no zlaw-hub, no NATS, no inter-agent communication yet. The zlaw-hub and inter-agent layer come in Phase 2.

Do not introduce zlaw dependencies into Phase 1 agent code. Design for it (e.g. use session IDs from day one), but don't couple to it.

---

## Key Architectural Decisions

- **Language**: Go. No other languages in core. Skill plugins can be any language via gRPC/IPC.
- **zlaw role**: Broker only — routes, verifies identity, audits. Does not plan or orchestrate.
- **Planner agent**: One designated agent receives user input and delegates to peers. Planning lives in an agent, not zlaw.
- **A2A routing**: All inter-agent messages via zlaw. Never direct agent-to-agent.
- **Config format**: TOML. Per-agent `agent.toml`, global `zlaw.toml`.
- **Personality**: `SOUL.md` + `IDENTITY.md` per agent. Hot-reloaded on file change.
- **Session model**: `map[sessionID → history]` from day one. Do not use a single global history.
- **Secrets**: Env-var injection only. Never plaintext in config files.
- **Plugin system**: Skill binaries over gRPC or net/rpc. Versioned contract defined in `plugins/`.
- **Message bus**: NATS, embedded in zlaw-hub binary by default.

---

## Agentic Loop (ReAct)

```
Input → Build context → LLM call
                            │
                    tool call? → YES → Execute → Append result → loop
                            │
                            NO → Emit response (done)
```

---

## Directory Layout

```
zlaw/
├── cmd/
│   ├── zlaw-hub/     # zlaw-hub binary
│   └── zlaw-agent/   # zlaw-agent binary
├── internal/
│   ├── agent/        # Agentic loop, history, context builder
│   ├── llm/          # LLM client abstraction + backends
│   ├── tools/        # Tool executor, registry, plugin IPC
│   ├── zlaw/          # zlaw core (Phase 2)
│   ├── nats/         # Embedded NATS (Phase 2)
│   ├── identity/     # Keypair management (Phase 2)
│   ├── adapters/     # Interface adapters (Telegram, CLI, HTTP)
│   └── config/       # Config loading, hot-reload
├── agents/
│   └── <agent-name>/  # agent.toml, SOUL.md, IDENTITY.md
├── plugins/          # Skill plugin contracts + binaries
├── zlaw.toml
└── README.md
```

---

## Coding Conventions

- Prefer explicit error handling over panic. No `log.Fatal` outside of `main()`.
- Interfaces first — define the interface before the implementation (especially for LLM client, tool executor, input/output adapters).
- No global state. Pass dependencies explicitly (no init() side effects for business logic).
- Context propagation — pass `context.Context` as first arg to all functions that do I/O or may be cancelled.
- Config structs are loaded once at startup and passed down; hot-reload fires a callback, does not mutate shared state unsafely.
- Structured logging with `slog` (stdlib). Every log line includes `agent`, `session_id`, and where applicable `trace_id`.
- Tests live alongside code (`_test.go`). Unit test the loop logic with a mock LLM client.

---

## Phase 1 Build Order

1. `internal/config` — load and watch `agent.toml`, `SOUL.md`, `IDENTITY.md`
2. `internal/llm` — LLM client interface + Anthropic backend
3. `internal/agent` — context builder, history manager, agentic loop
4. `internal/tools` — tool registry, executor stub (no real plugins yet)
5. `cmd/zlaw-agent` — wire everything, accept input from stdin
6. `internal/adapters/cli` — basic CLI adapter
7. `internal/tools/plugin` — real plugin IPC contract + example skill

Do not start Phase 2 (zlaw, NATS, adapters/telegram) until Phase 1 loop is working end-to-end with at least one real tool.

---

## References

- `ARCHITECTURE.md` — full system design, topology diagram, security model
- `PLANNING.md` — prioritized feature checklist, design decisions table, directory layout
