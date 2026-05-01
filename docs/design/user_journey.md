# User Journey

## Day 0 — First Install

```
zlaw init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton with first agent entry)
#            $ZLAW_HOME/credentials.toml (empty template, 0600)
#            $ZLAW_HOME/agents/<id>/ (full agent scaffold)

zlaw auth login --profile anthropic --type apikey
# Prompts for API key → saves to credentials.toml

zlaw hub start
# Starts hub, spawns registered agents
# Telegram is live. User can chat immediately.
```

## Day 1 — Growing the Agent Team

User runs:
```
zlaw ctl create agent id=coding role="Go developer"
```

- ctl scaffolds `$ZLAW_HOME/agents/coding/` with agent.toml + personality files
- ctl registers with hub, hub spawns process
- User: "Done. I can delegate coding work to it now."

## Day N — Operations

```bash
zlaw ctl get agents                # list all agents, status, last heartbeat
zlaw ctl get agent <id>           # get agent details
zlaw ctl get hub                  # hub health + per-agent status
zlaw ctl logs <id> [--follow]     # stream agent logs
zlaw ctl stop <id>                # stop agent process
zlaw ctl restart <id>             # restart agent process
zlaw ctl delete <id>              # stop + deregister
zlaw ctl configure <id> <key> <value>  # update runtime config
```

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent startup
- [hub.md](./hub.md) — hub control socket interface