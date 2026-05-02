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
| **Hub** | Communication broker | Routes agent-to-agent messages, external webhooks; enforces ACL |
| **ctl** | Human operator CLI | Scaffolds agent directories, manages secrets, talks to hub via control socket |

## Separation of Concerns

### Agent
- Self-contained process
- Owns filesystem under `$ZLAW_AGENT_HOME`
- Uses `AgentHome()` for all file paths
- Never calls `ZlawHome()`
- Never reads secrets (receives via env vars)
- Communicates with other agents via NATS only

### Hub
- Communication broker only
- Routes agent-to-agent and external-to-agent messages
- Enforces ACL at NATS layer
- Does NOT spawn or manage agent processes
- Does NOT access secrets

### ctl
- Scaffolds agent directories and files
- Owns and manages `secrets.toml`
- Injects secrets as env vars at agent spawn
- Talks to hub via Unix control socket

## Directory Structure

```
$ZLAW_HOME/                  # ctl-owned convention
├── zlaw.toml               # hub config + agent registry
├── secrets.toml            # operator-managed secrets (key-value pairs)
├── skills/                 # shared skills
├── run/                    # hub runtime (sockets, PIDs)
├── nats/                   # JetStream store
└── agents/<id>/            # ZLAW_AGENT_HOME for each agent

$ZLAW_AGENT_HOME/           # agent's self-contained root
├── agent.toml             # agent config (secret references only)
├── runtime.toml           # runtime overrides
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
Agent ← ctl (spawn context)
│   └── Via env vars: ZLAW_AGENT_HOME, ZLAW_NATS_URL, plus secret values
│   Note: ctl injects secrets at spawn via executor.
│
ctl → Hub (control socket)
│   └── Via Unix socket at $ZLAW_HOME/run/hub.sock
```

## Secret Injection Flow

```
1. ctl reads zlaw.toml → finds env_vars mapping for agent
2. ctl reads secrets.toml → looks up secret values
3. ctl injects env vars at spawn → agent receives only env vars
4. Agent resolves $VAR_NAME in agent.toml → uses value
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
| `zlaw.toml` | ctl | ctl + hub | NATS settings, agent registry (id → dir, env_vars mapping) |
| `secrets.toml` | ctl | ctl only | Secret key-value pairs |
| `agents/<id>/agent.toml` | ctl + agent | Agent | LLM backend, secret references, tool ACL, context budget |

**Key invariant: secrets never flow directly to agents as files.** ctl reads `secrets.toml`, injects referenced values as env vars at spawn time. Agents never read `secrets.toml`.

## See Also

- [constraints.md](./constraints.md) — hard rules and isolation levels
- [security.md](./security.md) — security model
- [user_journey.md](./user_journey.md) — day 0, day 1, day N workflows
- [agent_standalone.md](./agent_standalone.md) — standalone agent internals
- [hub.md](./hub.md) — hub internals
- [agent_secrets.md](./agent_secrets.md) — secrets design
- [agent_delegation.md](./agent_delegation.md) — P2P delegation design
- [llm_presets.md](./llm_presets.md) — LLM backend presets and config