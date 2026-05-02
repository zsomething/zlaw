# Command Line Interface

## Single Binary

`zlaw` is a single binary with subcommands.

## Subcommands

| Command | Purpose |
|---------|---------|
| `zlaw init` | Bootstrap `$ZLAW_HOME` or create agent workspace |
| `zlaw auth` | Manage secrets (add, list, remove) |
| `zlaw agent` | Run/serve/attach agents (standalone mode) |
| `zlaw ctl` | System lifecycle management (hub + agents) |

## Supervisor

**Supervisor** delegates to executor for spawning and managing agents.
See [ctl_supervisor.md](./ctl_supervisor.md) for detailed design.

## Agent Configuration

See [agent_lifecycle.md](./agent_lifecycle.md) for agent config details.

```toml
[[agents]]
id = "foo"
executor = "subprocess"
target = "local"
target_ssh = ""
restart_policy = "on-failure"
```

## Lifecycle Commands (`zlaw ctl`)

### System Lifecycle

```bash
zlaw ctl start         # Start NATS + hub + all agents
zlaw ctl stop          # Stop everything
```

### Agent Lifecycle

```bash
# Create new agent
zlaw ctl create agent foo --executor systemd --target ssh --target_ssh user@host

# Start/stop/restart
zlaw ctl agent start foo
zlaw ctl agent stop foo
zlaw ctl agent restart foo

# Delete (preserves home)
zlaw ctl agent delete foo

# Delete + prune (removes home)
zlaw ctl agent delete foo --prune
```

### Status Queries

```bash
# List all agents (via hub socket)
zlaw ctl get agents
zlaw ctl get agent foo

# Stream logs
zlaw ctl logs foo [--follow]
```

## Workspace Bootstrap (`zlaw init`)

```bash
zlaw init                # Full workspace: zlaw.toml + manager agent
zlaw init -a <name>     # Single agent scaffold only
```

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — supervisor design
- [agent_lifecycle.md](./agent_lifecycle.md) — executor + target matrix
- [user_journey.md](./user_journey.md) — day 0/1/N command usage
- [agent_secrets.md](./agent_secrets.md) — secrets design