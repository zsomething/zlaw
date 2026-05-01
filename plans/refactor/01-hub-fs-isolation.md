# Phase 1: Hub Filesystem Isolation

## Goal

Remove all direct filesystem access from Hub to agent directories. Hub must receive information at spawn time rather than reading it from disk.

## Problems

1. **Hub reads `agent.toml`** — `BuildCredentialEnv()` calls `config.LoadAgentConfigFile()` to discover auth profiles
2. **Hub reads `credentials.toml`** — reads from agent dir to load credential store
3. **Hub writes to agent dirs** — `agentConfigure` writes `runtime.toml` directly
4. **Hub uses `ZlawHome()` fallbacks** — `resolveAgentDir()` and supervisor fallback path

## Changes

### 1. Remove `BuildCredentialEnv` (remove `internal/hub/credentials.go`)

Current flow:
```
Hub reads agent.toml → discover auth profiles → load credentials.toml → write runtime creds
```

New flow (Phase 2):
```
Agent provides auth profile names at registration → Hub injects from global credentials.toml
```

**Delete:** `internal/hub/credentials.go` entirely

**Remove from caller** (`internal/hub/supervisor.go:buildCmd`):
```go
// Remove these lines:
credEnv, err := BuildCredentialEnv(entry)
// ... loop to inject into env
```

### 2. Extend `AgentEntry` to include auth profiles

In `internal/config/hub.go`:

```go
type AgentEntry struct {
    ID            string   `toml:"id"`
    Dir           string   `toml:"dir"`
    Binary        string   `toml:"binary"`
    RestartPolicy RestartPolicy `toml:"restart_policy"`
    Disabled      bool     `toml:"disabled,omitempty"`
    // NEW: Auth profiles this agent requires
    AuthProfiles  []string `toml:"auth_profiles,omitempty"`
}
```

Hub reads this from `zlaw.toml` — no agent filesystem access needed.

### 3. Hub reads from global `credentials.toml` (not per-agent)

Hub reads `$ZLAW_HOME/credentials.toml` (its own file, not agent's):

```go
// In supervisor.go, buildCmd:
profiles := entry.AuthProfiles // from zlaw.toml
store, err := credentials.LoadStore(filepath.Join(config.ZlawHome(), "credentials.toml"))
// Filter to needed profiles, write to run/credentials/<id>.toml
```

### 4. Remove `resolveAgentDir` fallback

In `internal/hub/supervisor.go`:

```go
// Before:
agentDir := resolveAgentDir(entry) // falls back to ZlawHome()/agents/<id>
if !filepath.IsAbs(agentDir) {
    agentDir = filepath.Join(config.ZlawHome(), agentDir)
}

// After:
agentDir := entry.Dir
if !filepath.IsAbs(agentDir) {
    // Log error and reject — no fallback
    return nil, fmt.Errorf("agent %q dir must be absolute, got %q", entry.ID, agentDir)
}
```

### 5. Remove agent directory write from control socket

In `internal/hub/control.go` — `agentConfigure` handler:
- **Before:** Writes `runtime.toml` directly to `$ZLAW_HOME/agents/<id>/`
- **After:** Forward to agent via hub inbox, or use agent's own mechanism

Option A (simpler): Agent subscribes to `agent.<id>.config` subject and handles writes itself.
Option B (remove hub inbox tool): Agent has a Unix socket endpoint for config changes.

For Phase 1, use a placeholder: config writes become a no-op with a warning logged. Full solution comes in Phase 2 when the hub-to-agent communication is formalized.

### 6. Remove `resolveAgentDir` function

Delete `internal/hub/credentials.go` — the function lives there. After removal, check for any remaining references.

## File Changes

| File | Action |
|------|--------|
| `internal/hub/credentials.go` | DELETE |
| `internal/config/hub.go` | Add `AuthProfiles []string` to `AgentEntry` |
| `internal/hub/supervisor.go` | Remove `BuildCredentialEnv` call; add auth profile loading from zlaw.toml; remove ZlawHome() fallback |
| `internal/hub/control.go` | Disable `agentConfigure` write, log warning |

## Verification

1. Hub no longer imports or calls any function that reads from agent directories
2. `zlaw.toml` entries must have absolute `dir` paths (enforced)
3. Credential injection still works (proven by integration test)
4. `go build ./...` passes
5. Existing tests pass

## Risks

- **Breaking change:** `zlaw.toml` entries without absolute `dir` will error instead of silently working
- **Migration needed:** Existing `zlaw.toml` with relative paths must be updated

## Migration

Add a one-time migration in `zlaw init` or `ctl` that converts relative paths to absolute:
```go
for i, entry := range cfg.Agents {
    if entry.Dir != "" && !filepath.IsAbs(entry.Dir) {
        cfg.Agents[i].Dir = filepath.Join(config.ZlawHome(), entry.Dir)
    }
}
cfg.Save()
```