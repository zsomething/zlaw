# Agent Lifecycle

## Overview

Agent lifecycle involves creating, spawning, monitoring, and stopping agent processes. ctl manages lifecycle; execution is handled by executor + target.

## Agent Configuration

Agents are configured in `zlaw.toml`:

```toml
[[agents]]
id = "assistant"
executor = "subprocess"
target = "local"
target_ssh = ""
restart_policy = "on-failure"
```

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

For local agents, the home directory structure:

```
$ZLAW_HOME/agents/{agent_id}/
├── agent.toml           # agent configuration
├── runtime.toml        # runtime overrides
├── SOUL.md             # personality
├── IDENTITY.md         # role definition
├── skills/             # per-agent skill files
├── sessions/           # conversation history
├── memories/           # long-term memory
└── workspace/         # agent's working directory
```

## Lifecycle Operations

### Create

1. ctl scaffolds agent directory
2. ctl creates executor-specific service (systemd unit, docker image, etc.)
3. ctl adds agent entry to zlaw.toml with executor/target/restart_policy

### Start

1. ctl reads all agents from zlaw.toml
2. ctl invokes executor.Start() for each agent
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
3. Executor removes service (systemd unit, docker container, etc.)
4. **Agent home is preserved** (sessions, memories intact)

### Delete --prune

1. ctl invokes executor.Stop()
2. ctl removes agent entry from zlaw.toml
3. ctl deletes agent home directory

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — ctl and executor design
- [hub_lifecycle.md](./hub_lifecycle.md) — hub's role at spawn
- [security.md](./security.md) — secret injection, subprocess filtering