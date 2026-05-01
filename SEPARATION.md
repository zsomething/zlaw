# Separation of Concerns — Violations

## Principles

1. **Agents** only access their own `ZLAW_AGENT_HOME`
2. **Hub** only knows agent ID, dir, binary, restart policy, disabled flag
3. **Agents** communicate via Hub NATS only
4. **Hub** is routing + ACL, nothing else
5. **Ctl** is human operator, accesses both hub (socket) and agent files
6. **Hub** can also be public interface (webhook) that routes to agents

---

## Current Violations

### 1. Hub reads `agent.toml` and `credentials.toml` directly

**File:** `internal/hub/credentials.go`

```go
func BuildCredentialEnv(entry config.AgentEntry) ([]string, error) {
    agentTOMLPath := filepath.Join(agentDir, "agent.toml")  // ← reads agent internals
    agentCfg, err := config.LoadAgentConfigFile(agentTOMLPath)
    // ...
    sourceCredsPath := filepath.Join(agentDir, "credentials.toml")  // ← reads agent internals
}
```

**Problem:** Hub reads agent configuration files directly from `ZLAW_AGENT_HOME`. This couples hub to agent internals.

**Should be:** Agent provides its config via registration message or a defined interface. Hub does not read from agent filesystem.

---

### 2. Hub uses `ZlawHome()` to resolve agent directories

**File:** `internal/hub/credentials.go`

```go
func resolveAgentDir(entry config.AgentEntry) string {
    if entry.Dir != "" {
        return entry.Dir
    }
    return filepath.Join(config.ZlawHome(), "agents", entry.ID)  // ← uses ZlawHome
}
```

**Files:** `internal/hub/credentials.go:18`, `internal/hub/control.go:411`

**Problem:** Hub depends on `ZlawHome()` to resolve agent directories when `entry.Dir` is empty. Hub should only work with absolute paths provided by ctl.

**Should be:** Hub never calls `ZlawHome()`. All agent dirs must be absolute.

---

### 3. Hub reads `runtime.toml` at configure time

**File:** `internal/hub/control.go` → `agentConfigure`

```go
func (cs *ControlSocket) agentConfigure(...) {
    agentDir := filepath.Join(config.ZlawHome(), "agents", p.ID)
    config.WriteRuntimeFieldToDir(agentDir, p.Key, p.Value)  // ← writes to agent dir
}
```

**Problem:** Hub directly modifies agent config files on disk.

**Should be:** Agent handles its own config changes. Hub relays the request to agent, or agent subscribes to config change events.

---

### 4. Hub's credential injection creates files in `run/`

**File:** `internal/hub/credentials.go`

```go
activeCredsPath := filepath.Join(runtimeCredsDir, entry.ID+".toml")
credentials.SaveStore(activeCredsPath, needed)  // ← writes to run/
```

**Problem:** Hub writes to `run/` directory. This is acceptable (hub-owned), but the path construction uses `ZlawHome()`.

**Should be:** Accept `--run-dir` flag explicitly instead of deriving from `ZlawHome()`.

---

### 5. Supervisor fallback uses `ZlawHome()` for agent directory

**File:** `internal/hub/supervisor.go:379-381`

```go
// Fall back to ZlawHome()-relative path only for legacy entries without a dir.
if !filepath.IsAbs(agentDir) {
    agentDir = filepath.Join(config.ZlawHome(), agentDir)
}
```

**Problem:** Allows non-absolute paths to resolve via `ZlawHome()`. Hub should require absolute paths.

**Should be:** Remove fallback. All `AgentEntry.Dir` must be absolute. Log warning or error on relative paths.

---

## What Is Correct ✅

### Agent uses only `AgentHome()`
```go
// internal/agent/history.go:188
return filepath.Join(config.AgentHome(), "sessions"), nil

// internal/agent/memory.go:41
return filepath.Join(config.AgentHome(), "memories"), nil
```
✅ Agent only accesses its own `ZLAW_AGENT_HOME`, never `ZlawHome()`.

### Hub injects `ZLAW_AGENT_HOME` env var
```go
// internal/hub/supervisor.go:403
env = SetEnv(env, "ZLAW_AGENT_HOME", agentDir)
```
✅ Hub tells agent where its home is, doesn't assume it.

### Hub spawns agent with minimal env
```go
env = SetEnv(env, "ZLAW_AGENT", entry.ID)
env = SetEnv(env, "ZLAW_NATS_URL", s.natsURL)
env = SetEnv(env, "ZLAW_CREDENTIALS_FILE", ...)  // from hub-generated file
```
✅ Hub only provides ID, NATS URL, and credentials. Does not inject workspace, sessions, etc.

### Ctl is the only one scaffolding agent files
```go
// cmd/zlaw/ctl.go
os.MkdirAll(agentHome, 0o700)
os.WriteFile(..., agent.toml)
os.WriteFile(..., SOUL.md)
os.WriteFile(..., IDENTITY.md)
```
✅ Only ctl creates agent directories and files.

### Agents communicate via NATS
```go
// internal/agent/hubclient.go
inboxSubject := fmt.Sprintf(inboxSubjectFmt, h.id)  // agent.<id>.inbox
```
✅ Agents use NATS subjects for all inter-agent communication.

### Hub provides ACL
```go
// internal/hub/acl.go
func agentPermissions(agentID string) *server.Permissions {
    inboxSubject := "agent." + agentID + ".inbox"
    // only this agent can subscribe to its own inbox
}
```
✅ Hub enforces ACL at NATS layer.

---

## Summary

| Concern | Violation | Severity |
|---------|-----------|----------|
| Hub reads agent.toml | `BuildCredentialEnv` | HIGH |
| Hub uses ZlawHome() | `resolveAgentDir`, supervisor fallback | HIGH |
| Hub writes runtime.toml | `agentConfigure` | HIGH |
| Hub logs warning on relative path | Supervisor fallback | LOW |

**Core issue:** Hub still needs to read `agent.toml` at spawn time for credential injection. The architectural fix is an interface where agent provides its auth profile requirements at registration time, eliminating hub's need to read from agent filesystem.

---

## Future: Hub as Public Interface

Hub can also serve as public interface (webhook/HTTP) that routes messages to agents. This is not yet implemented. When adding:

- Hub receives HTTP webhook → validates → publishes to NATS `agent.<id>.inbox`
- Hub's HTTP routes should be configurable (bind address, TLS, auth)
- All agent-to-agent routing still via NATS, not HTTP
