# CLAUDE.md — Project Context for Claude Code

File give Claude Code full context for project. Read ARCHITECTURE.md and PLANNING.md for full detail. File summarize essentials, give working conventions.

---

## What This Project Is

Multi-agent personal assistant platform in Go. Central **zlaw** process broker communication between autonomous **Agent** processes over embedded NATS message bus.

Primary use case: personal assistant (Telegram as main interface). Coding assistance nice-to-have.

---

## Current Implementation Phase

**Phase 1: Standalone Agent**

Build single zlaw-agent binary, run independently — no zlaw-hub, no NATS, no inter-agent yet. zlaw-hub and inter-agent layer come Phase 2.

No zlaw dependencies in Phase 1 agent code. Design for it (e.g. use session IDs from day one), but no coupling.

---

## Key Architectural Decisions

- **Language**: Go. No other languages in core. Skill plugins any language via gRPC/IPC.
- **zlaw role**: Broker only — routes, verifies identity, audits. No planning or orchestration.
- **Planner agent**: One designated agent receive user input, delegate to peers. Planning lives in agent, not zlaw.
- **A2A routing**: All inter-agent messages via zlaw. Never direct agent-to-agent.
- **Config format**: TOML. Per-agent `agent.toml`, global `zlaw.toml`.
- **Personality**: `SOUL.md` + `IDENTITY.md` per agent. Hot-reloaded on file change.
- **Session model**: `map[sessionID → history]` from day one. No single global history.
- **Secrets**: Env-var injection only. Never plaintext in config.
- **Plugin system**: Skill binaries over gRPC or net/rpc. Versioned contract in `plugins/`.
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

- Prefer explicit error handling over panic. No `log.Fatal` outside `main()`.
- Interfaces first — define interface before implementation (especially LLM client, tool executor, input/output adapters).
- No global state. Pass dependencies explicitly (no `init()` side effects for business logic).
- Context propagation — pass `context.Context` as first arg to all functions doing I/O or cancellable.
- Config structs loaded once at startup, passed down; hot-reload fires callback, no unsafe mutation of shared state.
- Structured logging with `slog` (stdlib). Every log line include `agent`, `session_id`, and where applicable `trace_id`.
- Tests alongside code (`_test.go`). Unit test loop logic with mock LLM client.

---

## Phase 1 Build Order

1. `internal/config` — load and watch `agent.toml`, `SOUL.md`, `IDENTITY.md`
2. `internal/llm` — LLM client interface + Anthropic backend
3. `internal/agent` — context builder, history manager, agentic loop
4. `internal/tools` — tool registry, executor stub (no real plugins yet)
5. `cmd/zlaw-agent` — wire everything, accept input from stdin
6. `internal/adapters/cli` — basic CLI adapter
7. `internal/tools/plugin` — real plugin IPC contract + example skill

No start Phase 2 (zlaw, NATS, adapters/telegram) until Phase 1 loop working end-to-end with at least one real tool.

---

## References

- `ARCHITECTURE.md` — full system design, topology diagram, security model
- `PLANNING.md` — prioritized feature checklist, design decisions table, directory layout