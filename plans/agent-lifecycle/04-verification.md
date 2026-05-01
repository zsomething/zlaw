# Phase 4: Verification

## Goal

Test full agent lifecycle end-to-end.

## Test Cases

### 4.1 Fresh Workspace Setup

```bash
# Clean start
rm -rf ~/.config/zlaw
zlaw init

# Verify structure
cat ~/.config/zlaw/zlaw.toml  # should have manager agent with executor/target fields
ls ~/.config/zlaw/agents/manager/  # should have all subdirs
```

### 4.2 Agent Lifecycle

```bash
# Start system
zlaw ctl start

# Check agents
zlaw ctl get agents  # should show manager connected

# Stop/restart agent
zlaw ctl agent stop manager
zlaw ctl agent start manager
zlaw ctl agent restart manager

# Stop system
zlaw ctl stop
```

### 4.3 Create Agent

```bash
zlaw ctl create agent assistant --executor subprocess
zlaw ctl start

# Verify agent running
zlaw ctl get agents  # should show both manager and assistant

# Delete agent
zlaw ctl agent delete assistant
# Verify assistant removed from zlaw.toml
```

### 4.4 Delete with Prune

```bash
zlaw ctl create agent temp
zlaw ctl agent stop temp
zlaw ctl agent delete temp --prune

# Verify home deleted
ls ~/.config/zlaw/agents/temp  # should fail
```

### 4.5 Multiple Agents

```bash
zlaw ctl create agent agent1 --executor subprocess
zlaw ctl create agent agent2 --executor subprocess
zlaw ctl start

# Verify all running
zlaw ctl get agents  # should show 3 agents

# Stop system
zlaw ctl stop

# Start again - all agents should restart
zlaw ctl start
zlaw ctl get agents  # should show all connected
```

### 4.6 Executor Behavior

#### Subprocess Executor
```bash
zlaw ctl create agent dev --executor subprocess
zlaw ctl start

# Stop system - agent should stop
zlaw ctl stop

# Start again - agent should restart
zlaw ctl start
```

#### Systemd Executor (when implemented)
```bash
zlaw ctl create agent prod --executor systemd
zlaw ctl start

# Verify systemd service exists
systemctl status zlaw-agent-prod

# Stop system - systemd should auto-restart
zlaw ctl stop
# Agent should restart via systemd
```

## Manual Testing Checklist

- [ ] `zlaw init` creates proper structure
- [ ] `zlaw ctl start` starts NATS + hub + all agents
- [ ] `zlaw ctl stop` stops everything
- [ ] `zlaw ctl get agents` shows correct status
- [ ] `zlaw ctl agent start` starts individual agent
- [ ] `zlaw ctl agent stop` stops individual agent
- [ ] `zlaw ctl agent restart` restarts individual agent
- [ ] `zlaw ctl agent delete` removes from zlaw.toml, preserves home
- [ ] `zlaw ctl agent delete --prune` removes everything
- [ ] Agents reconnect after hub restart
- [ ] Logs stream correctly

## Dependencies

- Phase 1: Executor abstraction
- Phase 2: ctl lifecycle commands
- Phase 3: Agent bootstrapping

## Notes

- NATS must be running for `zlaw ctl get agents` to work
- Tests should be idempotent (can run multiple times)
- Clean up after tests (rm -rf ~/.config/zlaw-test)
