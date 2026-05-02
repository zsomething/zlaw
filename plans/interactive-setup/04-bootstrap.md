# Phase 04: Bootstrap Screen

## Goal

Implement Bootstrap Zlaw Home screen using shared config package.

## Flow (Not Configured)

```
┌────────────────────────────────────────────┐
│  Create Zlaw Home?                         │
│                                             │
│  Path: /home/user/.config/zlaw             │
│                                             │
│  [Y] Create   [N] Cancel                   │
└────────────────────────────────────────────┘
```

## Flow (Already Configured)

```
┌────────────────────────────────────────────┐
│  Zlaw Home already exists at:              │
│  /home/user/.config/zlaw                   │
│                                             │
│  [R] Re-create   [K] Keep   [N] Cancel    │
└────────────────────────────────────────────┘
```

## Shared Config Management

Uses `internal/config/bootstrap.go` for file creation:

```go
import "github.com/zsomething/zlaw/internal/config"

// In bootstrapConfirm()
cfg := config.BootstrapConfig{
    Home:  m.state.Home,
    Force: true, // for re-create
}
if err := cfg.CreateZlawHome(); err != nil {
    m.errMsg = err.Error()
    return m, nil
}
```

This is shared with `zlaw init` — both entry points use the same logic.

## Creates (via config.BootstrapConfig.CreateZlawHome)

- `$ZLAW_HOME/zlaw.toml` (skeleton)
- `$ZLAW_HOME/secrets.toml` (empty, mode 0600)
- `$ZLAW_HOME/agents/` (directory)

## Implementation

```go
type bootstrapState struct {
    configured bool
    cursor     int
}

func (m *Model) bootstrapInit() {
    m.bootstrap = &bootstrapState{
        configured: m.state.IsConfigured(),
        cursor:     0,
    }
}
```

## State Refresh

After successful create/keep, reload state via `LoadState()` and return to menu.

## Verification

- Not configured: shows create confirmation
- Already configured: shows keep/re-create options
- Create creates all three files (via config package)
- Re-create prompts for confirmation
- Back returns to menu

## Files

| File | Change |
|------|--------|
| `cmd/zlaw/setup/bootstrap.go` | Bootstrap screen implementation (uses internal/config) |
| `internal/config/bootstrap.go` | Shared BootstrapConfig (already implemented) |
