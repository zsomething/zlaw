# Phase 5: Absolute Path Enforcement

## Goal

Remove all remaining `ZlawHome()` fallbacks throughout the codebase. All agent directories must be absolute paths passed explicitly.

## Design Rationale

From `docs/design/constraints.md`:
> Hub Must Not: Call `ZlawHome()` at runtime

The Hub should receive all paths as absolute, either from `zlaw.toml` or from the registration message. No dynamic resolution via `ZlawHome()`.

## Remaining Issues

After Phase 1-4, check for:

### 1. `internal/hub/control.go` — `agentConfigure` fallback

```go
// Before:
agentDir := filepath.Join(config.ZlawHome(), "agents", p.ID)

// After: Use absolute path from registry or zlaw.toml
agentDir := resolveAgentDirFromEntry(p.ID) // returns absolute or error
```

### 2. `internal/app/agent_serve.go` — workspace dir fallback

```go
// Before:
workspaceDir := os.Getenv("ZLAW_WORKSPACE")
if workspaceDir == "" {
    workspaceDir = filepath.Join(config.ZlawHome(), "workspace") // ← fallback
}

// After:
if workspaceDir == "" {
    // Agent should receive workspace path via env var or config
    return fmt.Errorf("workspace dir not set (ZLAW_WORKSPACE or workspace in agent.toml)")
}
```

### 3. `internal/hub/supervisor.go` — supervisor fallback (already addressed in Phase 1)

```go
// Phase 1 removes fallback:
// if !filepath.IsAbs(agentDir) {
//     agentDir = filepath.Join(config.ZlawHome(), agentDir)
// }

// Add validation at build time:
if !filepath.IsAbs(agentDir) {
    return nil, fmt.Errorf("agent %q dir must be absolute: %q", entry.ID, agentDir)
}
```

## Changes

### 1. Validate AgentEntry.Dir at Hub startup

In `internal/app/hub.go` — when loading config:

```go
for _, entry := range cfg.Agents {
    if entry.Dir == "" {
        return fmt.Errorf("agent %q has no dir set; absolute path required", entry.ID)
    }
    if !filepath.IsAbs(entry.Dir) {
        return fmt.Errorf("agent %q dir must be absolute, got %q", entry.ID, entry.Dir)
    }
}
```

### 2. Validate agent sends absolute path at registration

In `internal/hub/registry.go` — when agent registers:

```go
func (r *Registry) Register(entry RegistryEntry) {
    // Validate dir is absolute
    if entry.Dir != "" && !filepath.IsAbs(entry.Dir) {
        // Log warning but allow (agent might be running standalone)
        r.logger.Warn("agent registration with relative dir", "agent", entry.Name, "dir", entry.Dir)
    }
    // ... existing logic
}
```

### 3. Add migration for existing zlaw.toml

In `cmd/zlaw/init.go` or a dedicated migration command:

```go
// MigrateRelativeDirs migrates relative agent dirs to absolute.
func MigrateRelativeDirs(cfgPath string) error {
    cfg, err := config.LoadHubConfig(cfgPath)
    if err != nil {
        return err
    }
    
    changed := false
    for i, entry := range cfg.Agents {
        if entry.Dir != "" && !filepath.IsAbs(entry.Dir) {
            abs := filepath.Join(config.ZlawHome(), entry.Dir)
            cfg.Agents[i].Dir = abs
            changed = true
        }
    }
    
    if changed {
        // Save via AgentEntryEditor interface
        if err := cfg.RemoveAgent("*"); err != nil { // placeholder - need AddAgent approach
            // Actually use the existing save mechanism
        }
    }
    return nil
}
```

Better: add `SaveHubConfig()` method to `HubConfig`:

```go
func (c HubConfig) Save(path string) error {
    // serialize and write
}
```

Then migration can:
```go
cfg, _ := config.LoadHubConfig(path)
for i := range cfg.Agents {
    if !filepath.IsAbs(cfg.Agents[i].Dir) {
        cfg.Agents[i].Dir = filepath.Join(config.ZlawHome(), cfg.Agents[i].Dir)
    }
}
cfg.Save(path)
```

## File Changes

| File | Action |
|------|--------|
| `internal/config/hub.go` | Add `Save()` method to `HubConfig` |
| `internal/app/hub.go` | Add startup validation for absolute dirs |
| `internal/hub/control.go` | Remove `ZlawHome()` fallback in agentConfigure |
| `internal/app/agent_serve.go` | Remove workspace fallback |
| `cmd/zlaw/init.go` | Add migration for existing configs |

## Verification

1. `go build ./...` passes
2. All tests pass
3. Hub fails to start if any agent has relative dir
4. Migration converts existing relative paths to absolute

## Migration Path

1. On `zlaw init` or `hub start`, check for relative dirs
2. If found, print warning and offer to migrate:
   ```
   Warning: Agent "alice" has relative dir "agents/alice"
   Migrate to absolute path? [y/N]
   ```
3. On confirmation, convert to absolute
4. Save and continue

This ensures backward compatibility while pushing toward the design goal.

## Dependencies

- Phase 1: Removes `resolveAgentDir` fallback in supervisor
- Phase 5 cleans up remaining fallbacks discovered during implementation