# Overview: zlaw Architecture

## Design Principles

- **Hub is communication broker** — routes messages, enforces ACL. No task planning or agent management.
- **Agents are autonomous** — each runs own agentic loop independently. Any agent can receive user input if it has an adapter.
- **Adapters live in agents** — Telegram, CLI, etc. owned by agent. Hub owns no adapters.
- **Peer-to-peer delegation** — agents communicate directly over NATS. Hub provides routing + ACL only.
- **Secure by design** — agents verify identity via keypairs. Prompt injection mitigated at transport layer.
- **Simple ops** — single `zlaw` binary with subcommands. NATS embedded in hub by default.
- **Pluggable everywhere** — LLM backends + tool plugins swappable via config or binary plugins over IPC.

## Three Core Components

| Component | Role | Key Responsibility |
|-----------|------|---------------------|
| **Agent** | Autonomous executor | Runs agentic loop, owns its own filesystem, communicates via NATS |
| **Hub** | Communication broker | Routes agent-to-agent messages, external webhooks; enforces ACL
| **ctl** | Human operator CLI | Scaffolds agent directories, talks to hub via control socket |

## Separation of Concerns

### Agent
- Self-contained process
- Owns filesystem under `$ZLAW_AGENT_HOME`
- Uses `AgentHome()` for all file paths
- Never calls `ZlawHome()`
- Communicates with other agents via NATS only

### Hub
- Communication broker only
- Routes agent-to-agent and external-to-agent messages
- Enforces ACL at NATS layer
- Does NOT spawn or manage agent processes

### ctl
- Scaffolds agent directories and files
- Talks to hub via Unix control socket
- Accesses both hub and agent files

## Directory Structure

```
$ZLAW_HOME/                  # ctl-owned convention
├── zlaw.toml               # hub config + agent registry
├── credentials.toml        # global credentials
├── skills/                 # shared skills
├── run/                    # hub runtime (sockets, PIDs, injected creds)
├── nats/                   # JetStream store
└── agents/<id>/            # ZLAW_AGENT_HOME for each agent

$ZLAW_AGENT_HOME/           # agent's self-contained root
├── agent.toml             # agent config
├── runtime.toml           # runtime overrides
├── credentials.toml      # auth profiles (injected by hub)
├── cron.toml              # scheduled tasks
├── SOUL.md                # personality
├── IDENTITY.md            # role definition
├── skills/               # agent-specific skills
├── sessions/             # conversation history
├── memories/             # long-term memory
└── workspace/            # agent's writable CWD
```

## Communication Patterns

```
Agent → Agent (P2P)
│   └── Via NATS agent.<id>.inbox
│
Agent ← Hub (spawn context)
│   └── Via env vars: ZLAW_AGENT_HOME, ZLAW_NATS_URL, ZLAW_CREDENTIALS_FILE
│   Note: Hub does not spawn agents — that's ctl's responsibility.
│
ctl → Hub (control socket)
│   └── Via Unix socket at $ZLAW_HOME/run/control.sock
```

## NATS Subject Namespace

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `agent.<id>.inbox` | inbound | Tasks/messages for specific agent |
| `agent.<id>.outbox` | outbound | Responses/events (future) |
| `zlaw.webhook` | inbound | External webhook messages |
| `zlaw.registry` | bidirectional | Agent registration/heartbeat |
| `zlaw.audit` | inbound | Audit log (hub subscribes) |

## Configuration Boundaries

| File | Owned by | Read by | Contents |
|------|----------|---------|----------|
| `zlaw.toml` | Hub | Hub + ctl | NATS settings, agent registry (id → dir), hub keypair path, audit log path |
| `credentials.toml` | Hub | Hub at spawn time | LLM API keys, adapter tokens — injected into agents as env vars |
| `agents/<id>/agent.toml` | Agent | Hub (to spawn) + Agent | LLM backend, auth profile ref, tool ACL, context budget, isolation level |
| `agents/<id>/runtime.toml` | Agent (writes) | Agent + Hub watches | Dynamic overrides: model switching, flag toggles |

**Key invariant: credentials never flow directly to agents.** Hub reads `credentials.toml`, injects referenced profiles as env vars at spawn time. Agents never read `credentials.toml`.

## See Also

- [constraints.md](./constraints.md) — hard rules and isolation levels
- [security.md](./security.md) — security model
- [user_journey.md](./user_journey.md) — day 0, day 1, day N workflows
- [agent_standalone.md](./agent_standalone.md) — standalone agent internals
- [hub.md](./hub.md) — hub internals
- [agent_delegation.md](./agent_delegation.md) — P2P delegation design