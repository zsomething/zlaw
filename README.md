# zlaw

A multi-agent personal assistant platform written in Go.

A central **zlaw** process brokers communication between autonomous **Agent** processes over an embedded NATS message bus.

## Structure

```
zlaw/
├── cmd/
│   ├── zlaw-hub/     # Hub binary (Phase 2)
│   └── zlaw-agent/   # Standalone agent binary (Phase 1)
├── internal/
│   ├── config/       # Config loading, hot-reload
│   ├── llm/          # LLM client abstraction + backends
│   ├── agent/        # Agentic loop, history, context builder
│   ├── tools/        # Tool registry and executor
│   ├── adapters/cli/ # CLI input/output adapter
│   ├── zlaw/         # Hub core (Phase 2)
│   ├── nats/         # Embedded NATS (Phase 2)
│   └── identity/     # Keypair management (Phase 2)
├── agents/           # Per-agent configs (agent.toml, SOUL.md, IDENTITY.md)
├── plugins/          # Skill plugin contracts and binaries
└── zlaw.toml         # Global config
```

## Phase 1

Single standalone `zlaw-agent` binary. No hub, no NATS. See `ARCHITECTURE.md` and `PLANNING.md` for full detail.
