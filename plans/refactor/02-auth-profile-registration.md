# Phase 2: Auth Profile Registration

## Goal

Replace Hub reading `agent.toml` with Agent providing auth profile requirements at NATS registration. This eliminates the need for Hub to access agent filesystem entirely.

## Design

Currently (Phase 0):
```
Hub reads agent.toml → extract AuthProfiles → load credentials.toml → inject
```

New (Phase 2):
```
Agent registers with {auth_profiles: ["minimax-default", "telegram-bot"]}
Hub receives profiles from registration message
Hub reads global credentials.toml → injects only needed profiles
```

## Changes

### 1. Extend registration message to include auth profiles

In `internal/agent/hubclient.go` — `hubRegistration`:

```go
type hubRegistration struct {
    Name         string   `json:"name"`
    Version      string   `json:"version"`
    Capabilities []string `json:"capabilities"`
    Roles        []string `json:"roles"`
    // NEW:
    AuthProfiles []string `json:"auth_profiles,omitempty"`
}
```

### 2. Agent reads auth profiles from agent.toml (agent side only)

In `internal/agent/agent.go` or a new helper — agent reads its own config to discover needed profiles:

```go
// Agent reads its own agent.toml to discover required auth profiles
// This is the ONLY place agent accesses agent.toml
cfg, err := config.LoadAgentConfigFile(filepath.Join(agentHome, "agent.toml"))
profiles := collectAgentProfiles(cfg) // from LLM + memory + adapters
```

Agent includes these in its registration.

### 3. Hub caches auth profiles per agent

In `internal/hub/registry.go` — add to `RegistryEntry`:

```go
type RegistryEntry struct {
    Name         string
    Version      string
    Capabilities []string
    Roles        []string
    // NEW:
    AuthProfiles []string
    // ... existing fields
}
```

Hub stores auth profiles when agent registers.

### 4. Supervisor uses registry for auth profile lookup

In `internal/hub/supervisor.go` — `buildCmd`:

```go
// Before: BuildCredentialEnv(entry) // read from agent dir

// After:
registryEntry, ok := s.registry.Get(entry.ID)
var profiles []string
if ok {
    profiles = registryEntry.AuthProfiles
} else {
    profiles = entry.AuthProfiles // fallback to zlaw.toml (Phase 1 already added this)
}
```

### 5. Global credentials.toml lookup

Hub reads from `$ZLAW_HOME/credentials.toml` (its own file):

```go
globalCredsPath := filepath.Join(config.ZlawHome(), "credentials.toml")
store, err := credentials.LoadStore(globalCredsPath)

// Filter to only needed profiles
needed := filterProfiles(store, profiles)
```

Write filtered creds to `run/credentials/<agent-id>.toml` as before.

### 6. Agent stops receiving credentials via file path (optional cleanup)

After Phase 1+2, agent no longer needs `ZLAW_CREDENTIALS_FILE` env var because:
- Hub already injects profile values as env vars directly (design goal)

Wait — Phase 1 kept the file-based approach. Phase 2 keeps it but changes source. The design doc says env vars, but current implementation uses a file. 

For now: keep file-based (`ZLAW_CREDENTIALS_FILE`) but source from global credentials.toml, not per-agent file.

Future: Phase 3 could switch to direct env var injection as design specifies.

## File Changes

| File | Action |
|------|--------|
| `internal/agent/hubclient.go` | Add `AuthProfiles` to `hubRegistration` |
| `internal/hub/registry.go` | Add `AuthProfiles []string` to `RegistryEntry` |
| `internal/hub/supervisor.go` | Use registry for auth profile lookup; read from global credentials.toml |
| `internal/config/config.go` | Add `collectAgentProfiles()` helper |

## Verification

1. Agent registration message includes auth profiles
2. Hub receives and stores auth profiles from registration
3. Credential injection works from global credentials.toml
4. Old per-agent `credentials.toml` files are no longer read by hub
5. `go build ./...` passes
6. Existing tests pass

## Sequence

Phase 2 depends on Phase 1 because:
- Phase 1 removes `BuildCredentialEnv` which reads agent dir
- Phase 2 provides the replacement: agent-based auth profile discovery + hub-based credential injection

Without Phase 1, we can't cleanly test Phase 2 because the old code still exists.