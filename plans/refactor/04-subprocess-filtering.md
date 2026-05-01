# Phase 4: Subprocess Credential Filtering

## Goal

When agents spawn subprocesses (e.g., via bash tool), credential environment variables must be filtered out. This prevents a compromised subprocess from accessing the agent's secrets.

## Design

From `docs/design/security.md`:
> Planned implementation: subprocesses inherit only essential runtime vars (`ZLAW_AGENT_HOME`, `PATH`, etc.), excluding credential keys.

## Current State

The bash tool (`internal/tools/builtin/bash.go`) spawns subprocesses but does not filter environment variables. All credential env vars (injected by Hub) are passed through.

## Changes

### 1. Define essential env vars to preserve

In `internal/tools/builtin/bash.go`:

```go
// essentialEnvVars are the env vars that subprocesses are allowed to inherit.
// Credential keys (MINIMAX_*, ANTHROPIC_*, TELEGRAM_*, etc.) are excluded.
var essentialEnvVars = []string{
    "ZLAW_AGENT_HOME",
    "ZLAW_AGENT_ID",
    "ZLAW_NATS_URL",
    "ZLAW_LOG_LEVEL",
    "ZLAW_LOG_FORMAT",
    "ZLAW_NO_COLOR",
    "PATH",
    "HOME",
    "USER",
    "TERM",
    "PWD",
    // Add others as needed
}

// credentialKeyPrefixes are prefixes of env var names that contain secrets.
// These are filtered from subprocess environment.
var credentialKeyPrefixes = []string{
    "MINIMAX_",
    "ANTHROPIC_",
    "TELEGRAM_",
    "FIZZY_",
    "OPENAI_",
    "ZLAW_CREDENTIALS_FILE", // points to file with secrets
}
```

### 2. Filter environment before exec

```go
func filterEnv(env []string) []string {
    // Build set of allowed keys
    allowed := make(map[string]bool)
    for _, k := range essentialEnvVars {
        allowed[k] = true
    }
    
    // Check all env vars: keep if key is allowed, exclude if key starts with credential prefix
    var filtered []string
    for _, e := range env {
        idx := strings.IndexByte(e, '=')
        if idx < 0 {
            continue
        }
        key := e[:idx]
        
        // Skip if key matches credential prefix
        skip := false
        for _, prefix := range credentialKeyPrefixes {
            if strings.HasPrefix(key, prefix) {
                skip = true
                break
            }
        }
        if skip {
            continue
        }
        
        filtered = append(filtered, e)
    }
    
    return filtered
}
```

### 3. Apply filter in bash tool

```go
// Before: cmd.Env = os.Environ()
// After:
filteredEnv := filterEnv(os.Environ())
cmd.Env = filteredEnv
```

### 4. Make filter configurable per tool

Create a shared filter utility that tools can use:

```go
// internal/tools/envfilter/filter.go

package envfilter

var CredentialPrefixes = []string{
    "MINIMAX_",
    "ANTHROPIC_",
    "TELEGRAM_",
    "FIZZY_",
    "OPENAI_",
    "ZLAW_CREDENTIALS_FILE",
}

var EssentialVars = []string{
    "ZLAW_AGENT_HOME",
    "ZLAW_AGENT_ID",
    "ZLAW_NATS_URL",
    "ZLAW_LOG_LEVEL",
    "ZLAW_LOG_FORMAT",
    "ZLAW_NO_COLOR",
    "PATH",
    "HOME",
    "USER",
    "TERM",
    "PWD",
}

// Filter returns env with credential vars removed, essential vars kept.
func Filter(env []string) []string {
    allowed := make(map[string]bool)
    for _, k := range EssentialVars {
        allowed[k] = true
    }
    
    var filtered []string
    for _, e := range env {
        idx := strings.IndexByte(e, '=')
        if idx < 0 {
            continue
        }
        key := e[:idx]
        
        skip := false
        for _, prefix := range CredentialPrefixes {
            if strings.HasPrefix(key, prefix) {
                skip = true
                break
            }
        }
        if skip {
            continue
        }
        
        filtered = append(filtered, e)
    }
    
    return filtered
}
```

### 5. Update bash tool to use shared filter

```go
import "github.com/zsomething/zlaw/internal/tools/envfilter"

cmd.Env = envfilter.Filter(os.Environ())
```

## File Changes

| File | Action |
|------|--------|
| `internal/tools/envfilter/filter.go` | NEW — shared env filtering utility |
| `internal/tools/builtin/bash.go` | Apply env filter before subprocess spawn |

## Verification

1. Credential env vars are not visible in subprocess environment
2. Essential runtime vars are preserved
3. `env` command in bash tool shows no MINIMAX_*, ANTHROPIC_*, etc.
4. `go build ./...` passes
5. Existing tests pass

## Alternative Approaches

### Option A: Explicit allowlist (chosen)

Only pass through explicitly allowed vars. Simple, predictable. Risk: might miss needed vars.

### Option B: Explicit denylist

Strip only known credential prefixes. Less risky for missing vars, but relies on remembering all credential patterns.

Chose Option A because it's more secure by default — if a new credential pattern is added but not added to denylist, it's leaked.

## Dependencies

- None — Phase 4 is independent of Phases 1-3

## Non-Goals

- Filtering other channels (file access, network via proxy)
- Process capability dropping (seccomp, landlock)
- Container isolation (Phase 2 feature in design, not this plan)