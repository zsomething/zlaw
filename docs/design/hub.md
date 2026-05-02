# Hub: Communication Broker

## Overview

Hub is a communication broker. It routes messages between agents and provides external interfaces.

Hub lifecycle is managed by **ctl** via its executor subsystem. See [ctl_supervisor.md](./ctl_supervisor.md).

## Hub's Role

- **Message routing** — routes agent-to-agent messages via NATS
- **External interface** — optional webhook/HTTP endpoint to reach agents
- **ACL enforcement** — per-agent permissions at NATS layer
- **Audit logging** — logs all messages and tool calls

Hub does NOT:
- Spawn or stop agents
- Read agent configuration files
- Manage agent directories
- Access secrets (ctl handles secret injection)

## Components

| Component | Purpose |
|-----------|---------|
| NATS Server | Embedded message bus (JetStream enabled), managed by supervisor |
| NATS ACL | Per-agent publish/subscribe permissions |
| Audit Logger | Append-only structured log |
| Registry | Tracks connected agents (for routing, not management) |
| Webhook Handler | Optional HTTP endpoint for external messages |

## Startup

Managed by ctl via executor. See [agent_lifecycle.md](./agent_lifecycle.md).

## NATS ACL

All agents have equal P2P permissions:

| Permission | Subject Pattern | Purpose |
|------------|-----------------|---------|
| Subscribe | `zlaw.registry` | Agent registration/heartbeat |
| Subscribe | `agent.<id>.inbox` | Own inbox only |
| Subscribe | `_INBOX.>` | Reply subjects |
| Subscribe | `$JS.API.>` | JetStream API |
| Publish | `agent.*.inbox` | Delegate to any agent |
| Publish | `zlaw.registry` | Registration/heartbeat |
| Publish | `_INBOX.>` | Reply subjects |
| Publish | `$JS.API.>` | JetStream API |

## Subjects

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `agent.<id>.inbox` | inbound | Tasks for agent |
| `agent.<id>.outbox` | outbound | Responses from agent |
| `zlaw.registry` | bidirectional | Registration/heartbeat |
| `zlaw.audit` | inbound | Audit log |
| `zlaw.webhook` | inbound | External messages (optional) |

## Webhook (External Interface)

Optional HTTP endpoint for external services to send messages to agents:

```
POST /agent/<id>/message
Body: { "text": "hello", "session_id": "..." }
```

Hub validates and routes to `agent.<id>.inbox` via NATS.

## Agent Lifecycle

Managed by ctl via executor. See [ctl_supervisor.md](./ctl_supervisor.md).

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — ctl and executor design
- [hub_lifecycle.md](./hub_lifecycle.md) — hub lifecycle management
- [agent_lifecycle.md](./agent_lifecycle.md) — agent lifecycle
- [overview.md](./overview.md) — high-level architecture
- [agent_delegation.md](./agent_delegation.md) — P2P delegation
- [constraints.md](./constraints.md) — separation of concerns
- [agent_secrets.md](./agent_secrets.md) — secrets
- [llm_presets.md](./llm_presets.md) — LLM backend presets