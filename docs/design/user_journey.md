# User Journey

## Day 0 — First Install

```
zlaw init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton with first agent entry)
#            $ZLAW_HOME/credentials.toml (empty template, 0600)
#            $ZLAW_HOME/agents/<id>/ (full agent scaffold)

zlaw auth add --profile anthropic
# Prompts for API key → saves to credentials.toml

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
# Credentials management
zlaw auth add --profile anthropic          # Add credential
zlaw auth list                              # List all profiles
zlaw auth remove --profile <name>           # Remove profile

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
- [agent_credentials.md](./agent_credentials.md) — credentials design
- [security.md](./security.md) — security model