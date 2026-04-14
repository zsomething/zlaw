# CLAUDE.md — Project Context for Claude Code

Full context for project. Read ARCHITECTURE.md and PLANNING.md for detail. File summarize essentials + conventions.

---

## What This Project Is

Multi-agent personal assistant platform in Go. Central **zlaw** broker autonomous **Agent** processes over embedded NATS.

Primary use: personal assistant (Telegram). Coding assistance nice-to-have.

---

## Current Implementation Phase

**Phase 1: Complete. Phase 2: In Progress.**

Phase 1 (standalone agent) done — agent loop, tools, adapters, memory, cron, Telegram all working. Single `cmd/zlaw/` binary with subcommands (`run`, `serve`, `attach`, `auth`, `init`, `hub`).

Phase 2 focus: zlaw-hub binary, hub CLI bootstrap (`init`, `start`, `status`), hub core (NATS embed, agent supervisor, registry, identity verification). Hub internals partially implemented (`internal/hub/`).

Remaining Phase 1 gaps: plugin binary IPC, working memory, dry-run/sandbox mode.

---

## Key Architectural Decisions

- **Language**: Go. No other langs in core. Skill plugins any language via gRPC/IPC.
- **zlaw role**: Broker only — routes, verifies identity, audits. No planning/orchestration.
- **Manager agent**: One agent receives user input, delegates to peers. Routing in agent, not hub. Regular agent + hub-management tools + self-protection.
- **A2A routing**: All inter-agent msgs via zlaw. Never direct agent-to-agent.
- **Config format**: TOML. Per-agent `agent.toml`, global `zlaw.toml`.
- **Personality**: `SOUL.md` + `IDENTITY.md` per agent. Hot-reloaded on change.
- **Session model**: `map[sessionID → history]` from day one. No global history.
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
│   └── zlaw/         # single binary: run/serve/attach/auth/init/hub subcommands
├── internal/
│   ├── agent/        # agentic loop, history, context builder, memory, optimizer
│   ├── app/          # wiring for agent-run/serve/attach/hub modes
│   ├── llm/          # LLM client abstraction + Anthropic/OpenAI-compat backends
│   ├── tools/builtin/# file I/O, bash, glob, grep, web, HTTP, memory, cron, delegate
│   ├── hub/          # hub core: registry, supervisor, inbox, NATS, ACL, credentials
│   ├── nats/         # embedded NATS wrapper
│   ├── identity/     # keypair management
│   ├── adapters/     # CLI, daemon, Telegram
│   ├── session/      # session manager, event log, sink
│   ├── skills/       # skill file discovery + injection
│   ├── slashcmd/     # slash command parser + builtins
│   ├── push/         # push notification (Telegram)
│   ├── config/       # config loading, hot-reload
│   ├── cron/         # cron scheduler
│   ├── messaging/    # message types
│   ├── transport/    # Unix socket transport
│   ├── zlaw/         # zlaw core (Phase 2)
│   └── version/      # version info
├── agents/
│   └── <agent-name>/ # agent.toml, SOUL.md, IDENTITY.md, cron.toml
├── plugins/          # skill plugin contracts + binaries
├── zlaw.toml
└── README.md
```

---

## Coding Conventions

- Explicit error handling over panic. No `log.Fatal` outside `main()`.
- Interfaces first — define before implementation (LLM client, tool executor, adapters).
- No global state. Pass deps explicitly (no `init()` side effects for business logic).
- Pass `context.Context` as first arg to all I/O or cancellable funcs.
- Config loaded once at startup, passed down; hot-reload fires callback, no unsafe mutation.
- Structured logging with `slog` (stdlib). Every log: `agent`, `session_id`, `trace_id` where applicable.
- Tests alongside code (`_test.go`). Unit test loop with mock LLM client.

---

## Phase 2 Focus (current)

Active work: hub bootstrap CLI + hub core.

Next tasks (from PLANNING.md):
- `zlaw hub init` — generate `zlaw.toml`, `credentials.toml`, default manager agent scaffold
- `zlaw hub auth add` — add credential profiles
- `zlaw hub start` — start hub, embed NATS, spawn agents
- `zlaw hub status` — hub health + per-agent status
- `zlaw hub agent` subcommands — list/logs/restart/stop/remove
- Hub binary `serve` — NATS embed, agent supervisor, registry, identity verify, audit log
- Manager agent — gets hub-management tools, delegates to peers via NATS

See PLANNING.md for full checklist.

---

## References

- `ARCHITECTURE.md` — full system design, topology diagram, security model
- `PLANNING.md` — prioritized feature checklist, design decisions table, directory layout