# Hub: Internals

## Overview

Hub is a broker + process manager. It routes messages, enforces ACL, and supervises agent processes. It does NOT orchestrate tasks — that's the agents' responsibility.

## Components

| Component | Purpose |
|-----------|---------|
| Agent Supervisor | Spawns, stops, restarts agent processes |
| Agent Registry | Tracks connected agents, capabilities, health |
| NATS Server | Embedded message bus (JetStream enabled) |
| Control Socket | Unix socket for ctl commands |
| NATS ACL | Per-agent publish/subscribe permissions |
| Audit Logger | Append-only structured log |
| Inbox Handler | Hub management API (via control socket) |

## Startup

```
1. Load zlaw.toml                 → NATS config, agent registry
2. Start embedded NATS server     → with per-agent ACL
3. Start control socket           → at $ZLAW_HOME/run/control.sock
4. Spawn registered agents        → for each agent in registry
5. Subscribe to registry          → track agent health
```

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

Hub internal user (`_hub`): no restrictions.

## Agent Registration

Agents publish to `zlaw.registry` on connect and heartbeat:

```json
{
  "id": "alice",
  "version": "1.0.0",
  "capabilities": ["read", "write", "bash"],
  "roles": ["coding", "review"]
}
```

Hub maintains registry entry with:
- Status (connected/disconnected)
- Last heartbeat timestamp
- Capabilities
- Roles

## Credential Injection

At spawn time, hub:
1. Reads `agents/<id>/agent.toml` → discover auth profiles
2. Reads `agents/<id>/credentials.toml` → load profiles
3. Writes to `run/credentials/<id>.toml` → injected creds
4. Sets `ZLAW_CREDENTIALS_FILE` env var → points to injected file

This is a known violation — see [plans/separation.md](../plans/separation.md).

## Control Socket

Unix socket at `$ZLAW_HOME/run/control.sock` for ctl commands.

Requests:
- `get agents` — list all registered agents
- `get agent <id>` — get agent details
- `get hub` — hub health + per-agent status
- `stop <id>` — stop agent process
- `restart <id>` — restart agent process
- `disable <id>` — mark agent disabled (skip on restart)
- `enable <id>` — re-enable agent
- `delete <id>` — stop + deregister
- `logs <id>` — stream agent logs

## Subjects

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `agent.<id>.inbox` | inbound | Tasks for agent |
| `zlaw.registry` | bidirectional | Registration/heartbeat |
| `zlaw.hub.inbox` | inbound | Hub management (ctl only) |
| `zlaw.audit` | inbound | Audit log |

## See Also

- [overview.md](./overview.md) — high-level architecture
- [plans/separation.md](../plans/separation.md) — known violations