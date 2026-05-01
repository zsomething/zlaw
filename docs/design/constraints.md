# Constraints

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

## Execution Isolation Levels

Configurable per agent in `agent.toml`. Low to high:

| Level | Description |
|-------|-------------|
| `none` | Same user as hub, shared filesystem |
| `homedir` | Agent restricted to own virtual home directory |
| `user` | Agent runs as dedicated OS user (sudo drop) |
| `container` | Agent runs inside Docker container, connects to NATS by TCP address |

## See Also

- [overview.md](./overview.md) — separation of concerns
- [security.md](./security.md) — credential isolation, self-protection