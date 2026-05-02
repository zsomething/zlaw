# Phase 5: Secrets Refactor

## Goal

Refactor credential system to secrets model:
- `credentials.toml` → `secrets.toml`
- `AuthProfiles` → `env_vars` mapping
- ctl injects secrets, hub has no secret access

## Design Reference

See `docs/design/agent_credentials.md` — secrets model.

## Secrets Structure

```toml
# $ZLAW_HOME/secrets.toml (flat key-value pairs)
MINIMAX_API_KEY_DEV = "xxx"
MINIMAX_API_KEY_PROD = "yyy"
ANTHROPIC_API_KEY = "sk-ant-xxx"
TELEGRAM_BOT_TOKEN = "abc"
```

## Agent Mapping in zlaw.toml

```toml
[[agents]]
id = "assistant"
executor = "subprocess"
env_vars = [
  { name = "MINIMAX_API_KEY", from_secret = "MINIMAX_API_KEY_DEV" },
  { name = "TELEGRAM_BOT_TOKEN", from_secret = "TELEGRAM_BOT_TOKEN" }
]
```

## Implementation

### 5.1 Rename files and templates

**Files to modify:**
- `cmd/zlaw/init.go` — `credentialsTOMLTemplate` → `secretsTOMLTemplate`
- `cmd/zlaw/ctl.go` — agent.toml template update

```toml
# secrets.toml (flat key-value)
MINIMAX_API_KEY = "xxx"
ANTHROPIC_API_KEY = "sk-xxx"
TELEGRAM_BOT_TOKEN = "abc"
```

### 5.2 Update AgentConfig in executor

**File:** `internal/executor/executor.go`

```go
// EnvVarMapping maps env var name to secret name.
type EnvVarMapping struct {
    Name       string `toml:"name"`       // env var name injected to agent
    FromSecret string `toml:"from_secret"` // key in secrets.toml
}

type AgentConfig struct {
    // ... existing fields ...
    EnvVars []EnvVarMapping `toml:"env_vars"` // secrets to inject
}
```

### 5.3 Update secrets.toml read/load

**New file:** `internal/secrets/store.go`

```go
type Store map[string]string  // key → value

func LoadStore(path string) (Store, error)
func SaveStore(path string, Store) error
```

### 5.4 Update SubprocessExecutor secret injection

**File:** `internal/executor/subprocess.go`

```go
// Read secrets.toml
secretsPath := filepath.Join(config.ZlawHome(), "secrets.toml")
secrets, err := secrets.LoadStore(secretsPath)

// For each env_var in config
for _, ev := range cfg.EnvVars {
    value, ok := secrets[ev.FromSecret]
    if !ok {
        return fmt.Errorf("secret %q not found", ev.FromSecret)
    }
    env = setEnv(env, ev.Name, value)
}
```

### 5.5 Update agent.toml template

**File:** `cmd/zlaw/ctl.go`

```toml
[llm]
backend = "minimax"
model = "minimax-2.7"
secret = { api_key = "$MINIMAX_API_KEY" }

[[adapter]]
type = "telegram"
secret = { bot_token = "$TELEGRAM_BOT_TOKEN" }
```

### 5.6 Remove hub credential injection

**Files to modify:**
- `internal/hub/supervisor.go` — remove `injectCredentialsFromGlobal`
- `internal/app/hub.go` — remove credential-related code

Hub should have no secret access.

### 5.7 Update zlaw auth commands

**File:** `cmd/zlaw/agent_auth.go`

```bash
zlaw auth add --name MINIMAX_API_KEY    # Add secret (name only, value prompted)
zlaw auth list                           # List secret names (no values)
zlaw auth remove --name MINIMAX_API_KEY  # Remove secret
```

## Files to Modify

| File | Changes |
|------|---------|
| `internal/secrets/store.go` | New file for secrets.toml read/write |
| `internal/executor/executor.go` | Add EnvVarMapping, update AgentConfig |
| `internal/executor/subprocess.go` | Inject from secrets.toml via env_vars |
| `cmd/zlaw/init.go` | Rename template to secrets.toml |
| `cmd/zlaw/ctl.go` | Update agent.toml template with secret references |
| `cmd/zlaw/agent_auth.go` | Update to work with secrets.toml |
| `internal/hub/supervisor.go` | Remove credential injection |
| `internal/config/hub.go` | Remove AuthProfiles, add EnvVars |

## Verification

```bash
# Add secret
zlaw auth add --name MINIMAX_API_KEY
# Enter value when prompted

# List secrets (names only)
zlaw auth list

# Create agent with env_vars
cat >> zlaw.toml << 'EOF'
[[agents]]
id = "test"
executor = "subprocess"
env_vars = [
  { name = "MINIMAX_API_KEY", from_secret = "MINIMAX_API_KEY" }
]
EOF

# Verify agent receives env var
zlaw ctl start
# Agent should have MINIMAX_API_KEY env var set
```

## Dependencies

Requires Phase 1-4 to be complete.