# CLAUDE.md — Project Context

Read `docs/design/` for architecture. Read `plans/` for current work.

## What

Multi-agent personal assistant in Go. Central hub routes messages between autonomous agents over embedded NATS.

## Architecture

See `docs/design/`:
- `overview.md` — hub/agent/ctl separation
- `constraints.md` — hard rules
- `glossary.md` — terminology

## Current Work

See `plans/planning.md` for feature checklist.

## Key Decisions

- **Language**: Go only in core. Plugins any language via gRPC/IPC.
- **Hub**: Communication broker only. Routes messages, enforces ACL. No agent lifecycle management.
- **Agents**: Equal peers. Lifecycle via `ctl`, not hub. P2P delegation via NATS.
- **Credentials**: Env vars injected at spawn. No file path exposed.
- **Config**: TOML. Per-agent `agent.toml`, global `zlaw.toml`.
- **Personality**: `SOUL.md` + `IDENTITY.md` per agent.

## Directory Layout

```
cmd/zlaw/         # single binary: init/agent/hub/ctl
internal/
├── agent/        # agentic loop, memory, context
├── hub/          # NATS routing, ACL, registry
├── llm/          # LLM clients (OpenAI-compat, Anthropic)
├── tools/builtin/# built-in tools
└── adapters/     # telegram, cli, daemon
plans/           # implementation tracking
docs/design/     # architecture docs
docs/users/      # user docs
```

## Coding Conventions

- Error handling over panic. No `log.Fatal` outside `main()`.
- Interfaces first.
- No global state. Pass deps explicitly.
- `context.Context` first arg for I/O.
- Structured logging with `slog`.
- Parameter naming: `agentID` not `id`.
- Tests alongside code (`_test.go`).

## Documentation Map

```
plans/                    # Implementation tracking
├── planning.md           # Feature checklist
├── separation.md         # Violations tracking

docs/design/              # Architecture (goal state)
├── overview.md
├── constraints.md
├── glossary.md
├── agent_*.md           # Agent internals
├── hub.md
├── security.md
└── ...

docs/users/              # User docs
├── configuration.md
└── agent_tools.md
```