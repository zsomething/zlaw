# Phase 2: ctl Lifecycle Commands

## Goal

Add `ctl` commands for system and agent lifecycle management.

## Commands to Implement

### System Lifecycle

| Command | Description |
|---------|-------------|
| `zlaw ctl start` | Start NATS + hub + all agents |
| `zlaw ctl stop` | Stop everything |

### Agent Lifecycle

| Command | Description |
|---------|-------------|
| `zlaw ctl agent start <id>` | Start individual agent |
| `zlaw ctl agent stop <id>` | Stop individual agent |
| `zlaw ctl agent restart <id>` | Restart individual agent |
| `zlaw ctl agent delete <id>` | Stop + remove from zlaw.toml (preserve home) |
| `zlaw ctl agent delete <id> --prune` | Stop + remove + delete home |

## Design Reference

See `docs/design/command_line.md` and `docs/design/ctl_supervisor.md`

## Implementation

### 2.1 CtlCmd Structure

**File:** `cmd/zlaw/ctl.go`

```go
type CtlCmd struct {
    Start     CtlStartCmd     `cmd:"" help:"start NATS, hub, and all agents"`
    Stop      CtlStopCmd      `cmd:"" help:"stop NATS, hub, and all agents"`
    Get       CtlGetCmd       `cmd:"" help:"get resource info"`
    Agent     CtlAgentCmd     `cmd:"" help:"agent lifecycle management"`
    // ... existing commands
}
```

### 2.2 CtlStartCmd

Starts the entire system:
1. Start embedded NATS server
2. Start hub (via executor)
3. Start all agents in zlaw.toml (via executor)

```go
type CtlStartCmd struct{}

func (c *CtlStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
    // 1. Start NATS (if not running)
    // 2. Start hub
    // 3. Read agents from zlaw.toml
    // 4. For each agent, invoke executor.Start()
}
```

### 2.3 CtlStopCmd

Stops the entire system:
1. Stop all agents via executor
2. Stop hub
3. Stop NATS server

```go
type CtlStopCmd struct{}

func (c *CtlStopCmd) Run(ctx context.Context, logger *slog.Logger) error {
    // 1. Stop all agents via executor.Stop()
    // 2. Stop hub
    // 3. Stop NATS
}
```

### 2.4 CtlAgentCmd

Subcommand for agent-specific lifecycle:

```go
type CtlAgentCmd struct {
    Start   CtlAgentStartCmd   `cmd:"" help:"start agent"`
    Stop    CtlAgentStopCmd    `cmd:"" help:"stop agent"`
    Restart CtlAgentRestartCmd `cmd:"" help:"restart agent"`
    Delete  CtlAgentDeleteCmd  `cmd:"" help:"delete agent"`
}
```

### 2.5 CtlAgentStartCmd

Starts a single agent via executor:

```go
type CtlAgentStartCmd struct {
    ID string `arg:"true" help:"agent id"`
}

func (c *CtlAgentStartCmd) Run(ctx context.Context, logger *slog.Logger) error {
    // 1. Read agent config from zlaw.toml
    // 2. Get executor based on agent.Executor
    // 3. Call executor.Start()
}
```

### 2.6 CtlAgentDeleteCmd with --prune

```go
type CtlAgentDeleteCmd struct {
    ID     string `arg:"true" help:"agent id"`
    Prune  bool   `short:"p" help:"also delete agent home directory"`
}

func (c *CtlAgentDeleteCmd) Run(ctx context.Context, logger *slog.Logger) error {
    // 1. Stop agent via executor.Stop()
    // 2. Remove from zlaw.toml (HubConfig.RemoveAgent)
    // 3. If --prune, delete agent home directory
}
```

## Remove Existing Commands

The following commands should be removed or refactored:
- `CtlStopCmd` (currently stops single agent) → becomes `CtlAgentStopCmd`
- `CtlRestartCmd` (currently restarts single agent) → becomes `CtlAgentRestartCmd`
- `CtlDisableCmd` / `CtlEnableCmd` → deprecated, use delete/create
- `CtlCreateCmd` → kept but refactored

## Dependencies

Requires Phase 1 (Executor interface) to be complete.

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/zlaw/ctl.go` | Add Start/Stop/Agent commands, refactor existing |
| `internal/executor/` | Integrate with ctl |
| `internal/config/hub.go` | Already has AddAgent/RemoveAgent |

## Verification

```bash
zlaw ctl start
zlaw ctl get agents  # should show agents running
zlaw ctl agent stop <id>
zlaw ctl agent start <id>
zlaw ctl agent delete <id>
zlaw ctl agent delete <id> --prune
zlaw ctl stop
```
