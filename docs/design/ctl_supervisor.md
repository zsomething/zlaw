# ctl & Supervisor

## Overview

**ctl** is the operator CLI for managing the zlaw system. **Supervisor** delegates to executor to spawn and manage agents.

```
┌─────────────────────────────────────┐
│             ctl (CLI)               │
│                                     │
│  Supervisor                         │
│  └── Executor (subprocess/systemd)  │
│         or                          │
│  └── Executor (ssh/docker)          │
└─────────────────────────────────────┘
```

## Separation of Concerns

| Component | Role |
|-----------|------|
| **ctl** | Operator CLI. User-facing commands. Reads config, delegates to executor. |
| **Supervisor** | Coordinates executor selection and invocation. |
| **Executor** | Spawns, monitors, restarts agent processes. |

ctl is NOT concerned with:
- Process management details
- Restart logic
- Execution details (subprocess, systemd, docker, ssh)

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

Defines how to spawn and supervise:

```go
type Executor interface {
    // Start launches the agent
    Start(ctx context.Context, cfg AgentConfig) error
    
    // Stop terminates the agent
    Stop(ctx context.Context, id string) error
    
    // Status returns current state
    Status(ctx context.Context, id string) (Status, error)
    
    // Logs returns log stream
    Logs(ctx context.Context, id string) (io.ReadCloser, error)
}

// Subprocess executor: self-monitoring
type SubprocessExecutor struct{}

// Systemd executor: systemd services
type SystemdExecutor struct{}

// Docker executor: containers
type DockerExecutor struct{}
```

### Restart Policy

Defines restart behavior on exit. Systemd and docker have implicit autostart.


| Policy | Behavior |
|--------|----------|
| `always` | Restart regardless of exit |
| `on-failure` | Restart only on non-zero exit (default) |
| `never` | Do not restart |

**Executor-specific behavior:**

| Executor | Restart handling | After `ctl stop` |
|----------|------------------|------------------|
| `subprocess` | ctl monitors, restarts on `ctl start` | Stopped |
| `systemd` | systemd per policy (autostart) | Systemd restarts |
| `docker` | docker per policy (autostart) | Docker restarts |

### Target


Defines where to run:

| Target | Description | Notes |
|--------|-------------|-------|
| `local` | Same host, same user (default) | Default |
| `ssh` | Remote host via SSH | Requires `target_ssh` |

## Lifecycle Commands

### System Lifecycle

```bash
# Start NATS + hub + all agents
zlaw ctl start

# Stop everything
zlaw ctl stop
```

### Agent Lifecycle

```bash
# Create new agent
zlaw ctl create agent foo --executor systemd

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

## Communication

### ctl ↔ Hub

ctl connects to hub's control socket (`$ZLAW_HOME/run/hub.sock`):
- `ctl get agents` — query hub registry
- `ctl agent stop` — send stop command

### ctl ↔ Agent

ctl connects to agent's control socket (`$ZLAW_HOME/agents/<id>/agent.sock`):
- `ctl logs` — stream logs
- `ctl attach` — interactive session

For remote agents (target=ssh), communication over SSH requires separate design.

### ctl Never Uses NATS

Hard constraint. ctl talks to hub and agents via control sockets only.

## See Also

- [agent_lifecycle.md](./agent_lifecycle.md) — agent lifecycle, executor + target matrix
- [hub_lifecycle.md](./hub_lifecycle.md) — hub's role during lifecycle
- [command_line.md](./command_line.md) — CLI commands
- [llm_presets.md](./llm_presets.md) — LLM backend presets
- [agent_secrets.md](./agent_secrets.md) — secret injection