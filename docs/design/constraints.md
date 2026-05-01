# Constraints

## Hard Rules

### Agent Must
```
✅ Access only $ZLAW_AGENT_HOME
✅ Use AgentHome() for all file paths
✅ Communicate via NATS only
✅ Never call ZlawHome()
✅ Never read credentials (receives only env vars at spawn)
```

### Hub Must Not
```
❌ Read agent.toml directly
❌ Write to agent directories
❌ Call ZlawHome() at runtime
❌ Know about workspace, sessions, memories
❌ Own credentials (reads only at spawn for injection)
```

### ctl May
```
✅ Scaffold agent directories
✅ Create agent config files (agent.toml, SOUL.md, IDENTITY.md)
✅ Own and manage credentials.toml
✅ Talk to hub and agent via control sockets (never NATS)
✅ Access both hub and agent files
✅ Manage hub lifecycle (start, stop)
✅ Manage agent lifecycle (spawn, stop, restart, delete)
✅ Supervisor: execution abstraction for local/remote spawning
```

## Execution Models

Configurable per agent in `zlaw.toml`. See [agent_lifecycle.md](./agent_lifecycle.md) for details.

| Executor | Description | Restart |
|----------|-------------|---------|
| `subprocess` | Self-monitoring subprocess | ctl restarts on `ctl start` |
| `systemd` | systemd service | systemd restarts (implicit autostart) |
| `docker` | Docker container | docker restarts (implicit autostart) |

| Target | Description |
|--------|-------------|
| `local` | Same host (default) |
| `ssh` | Remote via SSH |

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — supervisor design
- [agent_lifecycle.md](./agent_lifecycle.md) — sandbox models
- [security.md](./security.md) — credential isolation, self-protection