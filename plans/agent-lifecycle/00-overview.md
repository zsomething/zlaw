# Implementation Plan: Agent Lifecycle

## Overview

Implement fully functional multi-agent system with ctl-managed lifecycle. Agents run independently (no delegation).

## Goals

1. **Fully functional agents** — agents can be created, started, stopped, restarted
2. **Fully functional agent lifecycle management** — via `ctl` command
3. **Executor abstraction ready** — supports subprocess (dev) and systemd (prod)
4. **Agent bootstrapping** — create agents with proper config

## Scope

### In Scope
- Executor abstraction (subprocess, systemd)
- ctl lifecycle commands (start/stop/restart/delete)
- Agent configuration (executor, target, restart_policy)
- Agent bootstrapping (zlaw init, ctl create)

### Out of Scope
- Agent delegation (P2P messaging)
- Docker executor
- SSH target (remote agents)
- Hub architecture changes

## Files

| File | Contents |
|------|----------|
| `00-overview.md` | This file |
| `01-executor.md` | Phase 1: Executor abstraction |
| `02-ctl-lifecycle.md` | Phase 2: ctl lifecycle commands |
| `03-bootstrapping.md` | Phase 3: Agent bootstrapping |
| `04-verification.md` | Phase 4: Testing |

## Current State

### Implemented
- `internal/executor/` package (interface + subprocess/systemd/docker stubs)
- `executor`, `target`, `target_ssh` fields in `AgentEntry`

### Not Implemented
- ctl start/stop commands
- ctl agent start/stop/restart/delete --prune
- Executor integration (currently in hub, needs to move to ctl)
- Updated templates

## Design References

- `docs/design/agent_lifecycle.md`
- `docs/design/ctl_supervisor.md`
- `docs/design/command_line.md`
