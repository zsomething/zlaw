# User Journey

> **See [interactive_setup.md](./interactive_setup.md) for the full onboarding design.**

## Quick Start

```bash
# Interactive TUI wizard (recommended for first-time setup)
zlaw setup

# Non-interactive for scripting
zlaw setup --non-interactive

# Run specific step only
zlaw setup --step init
zlaw setup --step llm
```

## Day 0 — First Install

The wizard guides through:
1. Initialize `zlaw_home` + config files
2. Create/update agents
3. Configure LLM provider (from presets)
4. Configure channel adapter
5. Set up secrets
6. Configure agent settings

Each step writes immediately; can quit and resume anytime.

### Manual Alternative

```bash
zlaw init
# Generates: $ZLAW_HOME/zlaw.toml (skeleton)
#            $ZLAW_HOME/secrets.toml (empty, mode 0600)
#            $ZLAW_HOME/agents/<id>/ (scaffold)

zlaw auth add --name MINIMAX_API_KEY
# Prompts for value → saves to secrets.toml

# Manual agent config (see llm_presets.md for inline copy pattern)
# Edit $ZLAW_HOME/agents/<id>/agent.toml

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
# Re-run setup wizard anytime to refine any part
zlaw setup --step llm --agent assistant  # Refine LLM for specific agent
zlaw setup --step secrets                # Add/update secrets

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

- [interactive_setup.md](./interactive_setup.md) — **Onboarding wizard design**
- [agent_lifecycle.md](./agent_lifecycle.md) — executor + target matrix
- [ctl_supervisor.md](./ctl_supervisor.md) — supervisor design
- [llm_presets.md](./llm_presets.md) — LLM presets + inline copy pattern
- [agent_secrets.md](./agent_secrets.md) — secrets + env var injection
- [channel_adapter.md](./channel_adapter.md) — channel adapter presets
- [security.md](./security.md) — security model