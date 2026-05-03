# Agent Lifecycle

## Overview

Agent lifecycle involves creating, spawning, monitoring, and stopping agent processes. ctl manages lifecycle; execution is handled by executor + target.

## Agent Configuration

Agents are configured in `zlaw.toml`:

```toml
[[agents]]
id = "assistant"
config = "agent-assistant.toml"  # path relative to $ZLAW_HOME
executor = "subprocess"
target = "local"
restart_policy = "on-failure"

[[agents.env_vars]]
name = "MINIMAX_API_KEY"
from_secret = "MINIMAX_API_KEY"
```

**Key design decision:** Agent configuration (llm, adapter, etc.) is owned by ctl and stored in `$ZLAW_HOME/agent-{id}.toml`, not in the agent's home directory. This enables sandboxed executors to isolate config from the agent process. See [agent config ownership refactoring](../plans/refactor/06-agent-config-ownership.md).

### Config File Location

By convention, agent configs live at `$ZLAW_HOME/agent-{id}.toml`. The path is configurable in `zlaw.toml`, but convention is preferred for consistency.

### Executor

Defines how the agent process is spawned and supervised:

| Executor | Description | Use Case |
|----------|-------------|----------|
| `subprocess` | Self-monitoring subprocess | Development |
| `systemd` | systemd service | Production |
| `docker` | Docker container | Containerized deployment |

### Restart Policy

Defines restart behavior on exit:

| Policy | Behavior |
|--------|----------|
| `always` | Restart regardless of exit |
| `on-failure` | Restart only on non-zero exit (default) |
| `never` | Do not restart |

**Executor-specific behavior:**

| Executor | Restart handling | Agent state after `ctl stop` |
|----------|------------------|-----------------------------|
| `subprocess` | ctl monitors and restarts on `ctl start` | Stopped until next `ctl start` |
| `systemd` | systemd restart per policy (implicit autostart) | Systemd restarts independently |
| `docker` | docker restart per policy (implicit autostart) | Docker restarts independently |

### Target

Defines where the agent runs:

| Target | Description | Notes |
|--------|-------------|-------|
| `local` | Same host, same user (default) | Default |
| `ssh` | Remote host via SSH | Requires `target_ssh` |

### Executor + Target Matrix

Feasibility of each combination:

| Executor | Target | Feasibility | Notes |
|----------|--------|-------------|-------|
| subprocess | local | ✅ Yes | Dev mode |
| subprocess | ssh | ⚠️ TBD | How does ctl monitor remote subprocess? |
| systemd | local | ✅ Yes | Production on same host |
| systemd | ssh | ⚠️ TBD | SSH + systemd on remote |
| docker | local | ⚠️ TBD | Docker on same host |
| docker | ssh | ⚠️ TBD | Docker on remote host |

⚠️ TBD combinations require separate design for:
- ctl control socket communication
- Health monitoring over remote connections
- Credential injection over SSH

## Agent Home

For local agents, the home directory contains user content and runtime data, **NOT configuration**:

```
$ZLAW_HOME/agents/{agent_id}/
├── runtime.toml        # runtime overrides
├── SOUL.md             # personality (user content, hot-reload)
├── IDENTITY.md         # role definition (user content, hot-reload)
├── skills/             # per-agent skill files
├── sessions/           # conversation history
├── memories/           # long-term memory
└── workspace/          # agent's working directory
```

**Note:** Agent configuration (llm, adapter) is stored in `$ZLAW_HOME/agent-{id}.toml` and managed by ctl.

## Lifecycle Operations

### Create

1. ctl creates agent config in `$ZLAW_HOME/agent-{id}.toml`
2. ctl scaffolds agent directory
3. ctl creates executor-specific service (systemd unit, docker image, etc.)
4. ctl adds agent entry to zlaw.toml with executor/target/restart_policy and `config` path

### Start

1. ctl reads all agents from zlaw.toml
2. ctl invokes executor.Start() for each agent (passing config path via env/arg)
3. All agents in zlaw.toml start

### Stop

1. ctl invokes executor.Stop()
2. Executor terminates agent
3. Hub unregisters agent
4. Agent restarts on next `zlaw ctl start`

### Restart

1. ctl invokes executor.Stop()
2. ctl invokes executor.Start()

### Delete

1. ctl invokes executor.Stop()
2. ctl removes agent entry from zlaw.toml
3. ctl removes agent config from `$ZLAW_HOME/agent-{id}.toml`
4. Executor removes service (systemd unit, docker container, etc.)
5. **Agent home is preserved** (sessions, memories intact)

### Delete --prune

1. ctl invokes executor.Stop()
2. ctl removes agent entry from zlaw.toml
3. ctl removes agent config from `$ZLAW_HOME/agent-{id}.toml`
4. ctl deletes agent home directory

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — ctl and executor design
- [hub_lifecycle.md](./hub_lifecycle.md) — hub's role at spawn
- [security.md](./security.md) — secret injection, subprocess filtering
- [llm_presets.md](./llm_presets.md) — LLM backend presets and agent config
- [Agent Config Ownership Refactoring](../plans/refactor/06-agent-config-ownership.md) — detailed refactoring plan