# Architecture Principles

## Core Concepts

| Concept | Description |
|---------|-------------|
| **ZLAW_HOME** | Convention for local setups. ctl-owned. Hub only knows `--config` and `--run-dir` paths. |
| **ZLAW_AGENT_HOME** | Agent's self-contained root. Set via env var. Agent reads this for all its files. |
| **Hub** | Routing + ACL. Knows only: agent ID, dir, binary, restart policy, disabled flag. |
| **Agent** | Self-contained process. Owns its own filesystem under `ZLAW_AGENT_HOME`. Communicates via NATS only. |
| **ctl** | Human operator. Scaffolds agent dirs, talks to hub via socket. |

## Directory Layout

```
$ZLAW_HOME/                  (ctl-owned convention)
  zlaw.toml                   # hub config + agent registry
  credentials.toml            # global fallback
  skills/                    # shared skills
  run/                        # hub runtime (sockets, PIDs, injected creds)
  nats/                       # JetStream store
  agents/<id>/               # ZLAW_AGENT_HOME for each agent

$ZLAW_AGENT_HOME/            # agent's self-contained root
  agent.toml                  # agent config
  runtime.toml               # runtime overrides
  credentials.toml           # auth profiles
  cron.toml                  # scheduled tasks
  SOUL.md                    # personality
  IDENTITY.md                # role definition
  skills/                    # agent-specific skills
  sessions/                  # conversation history
  memories/                  # long-term memory
  workspace/                 # agent's writable CWD
```

## Separation of Concerns

### Agent ✅
- Only reads/writes files under `ZLAW_AGENT_HOME`
- Uses `AgentHome()` (from `ZLAW_AGENT_HOME` env var)
- Never calls `ZlawHome()`
- Communicates with other agents via NATS only
- Receives identity info (ID, roles) via env vars and sticky blocks

### Hub ✅ (with exceptions)
- Knows only: ID, dir, binary, restart policy, disabled flag
- Injects `ZLAW_AGENT_HOME` env var when spawning
- Provides ACL at NATS layer
- Only provides ID, NATS URL, and credentials to agents

### ctl ✅
- Human operator layer
- Scaffolds agent directories and files
- Talks to hub via Unix control socket
- Creates `zlaw.toml` and manages agent registry

### Hub as Public Interface (future)
- May serve HTTP/webhook interface
- Routes external messages to agents via NATS
- All agent-to-agent routing via NATS, not HTTP

## Hard Rules

### Agent Must
```
✅ Access only $ZLAW_AGENT_HOME
✅ Use AgentHome() for all file paths
✅ Communicate via NATS only
✅ Never call ZlawHome()
```

### Hub Must Not
```
❌ Read agent.toml directly
❌ Read credentials.toml directly
❌ Write to agent directories
❌ Call ZlawHome() at runtime
❌ Know about workspace, sessions, memories
```

### ctl May
```
✅ Scaffold agent directories
✅ Create agent config files (agent.toml, SOUL.md, IDENTITY.md)
✅ Talk to hub via control socket
✅ Access both hub and agent files
```

## Current Violations

See [separation.md](../plans/separation.md) for detailed violations tracking.

## Implementation Status

See [separation.md](../plans/separation.md) for completion tracking.

## References

- [separation.md](../plans/separation.md) - Detailed violations and future fixes
- [ctl_plan.md](../plans/ctl_plan.md) - ctl subcommand implementation plan
- [agent_portability.md](../plans/agent_portability.md) - Original portability design
