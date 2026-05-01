# Command Line Interface

## Single Binary

`zlaw` is a single binary with subcommands.

## Subcommands

| Command | Purpose | Runs as |
|---------|---------|---------|
| `zlaw init` | Bootstrap `$ZLAW_HOME` or create agent workspace | Any |
| `zlaw agent` | Run/serve/attach agents, manage auth | Standalone |
| `zlaw hub` | Start hub, check status | Controller |
| `zlaw ctl` | Operational commands (requires hub running) | Operator |

## See Also

- [user_journey.md](./user_journey.md) — day 0/1/N command usage
- [hub.md](./hub.md) — control socket interface