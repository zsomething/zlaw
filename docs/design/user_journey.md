# User Journey

## Day 0 — First Install

```
zlaw init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton with first agent entry)
#            $ZLAW_HOME/secrets.toml (empty template, 0600)
#            $ZLAW_HOME/agents/<id>/ (full agent scaffold)

zlaw auth add --name MINIMAX_API_KEY
# Prompts for value → saves to secrets.toml

zlaw ctl start
# Starts NATS + hub + all agents
# Agents connect. User can delegate work.
```

## Day 1 — Growing the Agent Team

User runs:
```
zlaw ctl create agent coding --role "Go developer"
```

- ctl scaffolds `$ZLAW_HOME/agents/coding/` with agent.toml + personality files
- ctl adds agent entry to zlaw.toml
- ctl starts the agent (default executor=subprocess, target=local)

To configure for production:
```toml
[[agents]]
id = "coding"
executor = "systemd"
restart_policy = "on-failure"
```

## Day N — Operations

```bash
# Secrets management
zlaw auth add --name MINIMAX_API_KEY_DEV     # Add secret
zlaw auth list                                # List secret names (no values)
zlaw auth remove --name <name>               # Remove secret

# System management
zlaw ctl start                              # Start NATS + hub + agents
zlaw ctl stop                               # Stop everything

# Agent management (requires hub running)
zlaw ctl get agents                         # list all agents, status
zlaw ctl get agent <id>                    # get agent details
zlaw ctl logs <id> [--follow]               # stream agent logs
zlaw ctl agent start <id>                  # start agent
zlaw ctl agent stop <id>                   # stop agent
zlaw ctl agent restart <id>                # restart agent
zlaw ctl agent delete <id>                # stop + remove from zlaw.toml (preserve home)
zlaw ctl agent delete <id> --prune        # stop + remove + delete home directory
```

## See Also

- [agent_lifecycle.md](./agent_lifecycle.md) — executor + target matrix
- [ctl_supervisor.md](./ctl_supervisor.md) — supervisor design
- [agent_secrets.md](./agent_secrets.md) — secrets design
- [security.md](./security.md) — security model