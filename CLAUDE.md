# CLAUDE.md — Project Context for Claude Code

Full context for project. Read key docs and plans for detail.

---

## What This Project Is

Multi-agent personal assistant platform in Go. Central **zlaw** broker autonomous **Agent** processes over embedded NATS.

Primary use: personal assistant (Telegram). Coding assistance nice-to-have.

---

## Current Implementation Phase

**Phase 1: Complete. Phase 2: In Progress.**

Phase 1 (standalone agent) done — agent loop, tools, adapters, memory, cron, Telegram all working. Single `cmd/zlaw/` binary with subcommands (`run`, `serve`, `attach`, `auth`, `ctl`, `hub`).

Phase 2 focus: hub binary, agent management, P2P delegation.

---

## Key Architectural Decisions

- **Language**: Go. No other langs in core. Skill plugins any language via gRPC/IPC.
- **Hub role**: Broker + process manager — routes, verifies identity, audits. No planning/orchestration.
- **Agents**: All equal peers. Lifecycle tools (create/stop/configure) are CLI-only via `ctl`. P2P delegation via NATS.
- **Config format**: TOML. Per-agent `agent.toml`, global `zlaw.toml`.
- **Personality**: `SOUL.md` + `IDENTITY.md` per agent. Hot-reloaded on change.
- **Session model**: `map[sessionID → history]` from day one. No global history.
- **Secrets**: Env-var injection only. Never plaintext in config.
- **Plugin system**: Skill binaries over gRPC or net/rpc. Versioned contract in `plugins/`.
- **Message bus**: NATS, embedded in hub binary by default.

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
│   └── zlaw/         # single binary: init/agent/hub/ctl subcommands
├── internal/
│   ├── agent/        # agentic loop, history, context builder, memory, optimizer
│   ├── app/          # wiring for agent-run/serve/attach/hub modes
│   ├── llm/          # LLM client abstraction + Anthropic/OpenAI-compat backends
│   ├── tools/builtin/# file I/O, bash, glob, grep, web, HTTP, memory, cron, delegate
│   ├── hub/          # hub core: registry, supervisor, inbox, NATS, ACL, credentials
│   ├── nats/         # embedded NATS wrapper
│   ├── adapters/     # CLI, daemon, Telegram
│   ├── config/       # config loading, hot-reload
│   └── ...
├── agents/
│   └── <id>/ # agent.toml, SOUL.md, IDENTITY.md, cron.toml
├── plans/            # living implementation plans (evolves fast)
├── docs/
│   ├── design/       # architecture, principles, design decisions
│   └── users/        # user-facing documentation
└── plugins/          # skill plugin contracts
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
- Parameter naming: `agentID` not `id` (Go keyword).

---

## Documentation Map

```
plans/                    # Living plans — implementation tracking, evolves fast
├── planning.md           # Feature checklist, prioritized
├── agent_portability.md # ZLAW_AGENT_HOME consolidation design
├── ctl_plan.md          # ctl subcommand implementation
└── web_ui_plan.md       # Web UI implementation plan

docs/design/              # Architecture & design — the goal state
├── architecture.md      # Full system design, topology, security model
└── architecture_principles.md # Hard rules, violations, separation of concerns

docs/users/               # User-facing documentation
├── configuration.md     # Configuration guide
└── tools.md              # Built-in tools reference

SEPARATION.md            # Current separation of concerns violations (internal)
```

---

## Next Tasks

See `plans/planning.md` for full feature checklist.

- Hub binary: NATS embed, agent supervisor, registry
- P2P delegation: agent-to-agent via NATS
- Identity: NKeys keypair generation + hub verification

See `docs/design/architecture.md` for system design. See `docs/design/architecture_principles.md` for architectural rules.