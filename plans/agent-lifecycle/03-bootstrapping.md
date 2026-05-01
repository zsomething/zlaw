# Phase 3: Agent Bootstrapping

## Goal

Update agent templates and ctl create command to support executor/target fields.

## Design Reference

See `docs/design/agent_lifecycle.md` — Agent Configuration section.

## Current State

### zlaw init templates

**File:** `cmd/zlaw/init.go`

Current manager agent config in zlaw.toml:
```toml
[[agents]]
name = "manager"
dir = "/path/to/agent"
```

Missing fields: `executor`, `target`, `restart_policy`

### ctl create templates

**File:** `cmd/zlaw/ctl.go`

Current agent.toml template:
```toml
[agent]
id = %q
description = ""
```

Missing fields and structure.

## Implementation

### 3.1 Update zlawTOMLTemplate

**File:** `cmd/zlaw/init.go`

Add executor/target fields:

```toml
[[agents]]
id = "manager"
dir = %q
executor = "subprocess"
target = "local"
restart_policy = "on-failure"
```

### 3.2 Update agentTOMLTemplate

**File:** `cmd/zlaw/ctl.go`

Add executor/target fields:

```toml
[agent]
id = %q
description = ""

[llm]
# ...

# Execution configuration
[agent.executor]
executor = "subprocess"
target = "local"
restart_policy = "on-failure"
```

Or inline in [agent] section (TOML allows this):

```toml
[agent]
id = %q
description = ""
executor = "subprocess"
target = "local"
restart_policy = "on-failure"
```

### 3.3 Add --executor Flag to ctl create

**File:** `cmd/zlaw/ctl.go`

```go
type CtlCreateAgentCmd struct {
    Name      string `arg:"true" help:"agent id"`
    AgentHome string `name:"agent-home" help:"absolute path for agent home"`
    Executor  string `name:"executor" help:"executor type (subprocess, systemd, docker)"`
    Target    string `name:"target" help:"target (local, ssh)"`
    Start     bool   `help:"start agent after creation"`
}

func (c *CtlCreateAgentCmd) Run(...) error {
    executor := c.Executor
    if executor == "" {
        executor = "subprocess"
    }
    target := c.Target
    if target == "" {
        target = "local"
    }
    // Use executor/target in template
}
```

### 3.4 Update zlaw init Agent Template

**File:** `cmd/zlaw/init.go`

Current agent template:
```toml
const ctlIdentityMDTemplate = `# Identity

Your name is %s.
`
```

Keep as is, but ensure consistency with ctl create.

### 3.5 Add Workspace Subdirs

Both `zlaw init` and `ctl create` should create:
```
$AGENT_HOME/
├── agent.toml
├── credentials.toml
├── SOUL.md
├── IDENTITY.md
├── skills/
├── sessions/
├── memories/
└── workspace/
```

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/zlaw/init.go` | Update zlawTOMLTemplate, add subdirs |
| `cmd/zlaw/ctl.go` | Update agentTOMLTemplate, add --executor flag |

## Verification

```bash
zlaw init
cat $ZLAW_HOME/zlaw.toml  # should have executor/target fields

zlaw ctl create agent foo --executor systemd
cat $ZLAW_HOME/agents/foo/agent.toml  # should have executor/target fields

ls $ZLAW_HOME/agents/foo/  # should have all subdirs
```

## Dependencies

Phase 2 (ctl lifecycle commands) can be developed in parallel, but this phase is independent.

## Backward Compatibility

Old zlaw.toml entries without executor/target should default to:
- `executor = "subprocess"`
- `target = "local"`
- `restart_policy = "on-failure"`
