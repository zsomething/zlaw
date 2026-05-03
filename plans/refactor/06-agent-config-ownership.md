# 06: Agent Config Ownership Refactoring

## Goal

Move agent configuration (`llm`, `adapter`, etc.) from per-agent home directory to be owned and managed by ctl, similar to how `zlaw.toml` and `secrets.toml` are owned.

## Rationale

### Current State
- Agent config stored in `$ZLAW_HOME/agents/<id>/agent.toml`
- Agent can read its own config file
- Config is scoped to agent's home directory

### Problems
1. **Sandbox leakage**: In sandboxed environments, agent can read its own config via filesystem
2. **Inconsistent ownership**: Hub config (`zlaw.toml`), secrets (`secrets.toml`) owned by ctl, but agent config owned by... the agent itself?
3. **Secret references in agent home**: Even if agent only gets env vars at runtime, config file still exposes `$VAR_NAME` references

### Benefits
1. **Sandboxed isolation**: Agent receives config via process arguments or env var, not filesystem access
2. **Centralized management**: All configs under ctl control
3. **Clean separation**: Agent is just a process, doesn't need filesystem knowledge

## Design

### Config File Location

By convention, agent configs live at `$ZLAW_HOME/agent-{id}.toml`:

```
$ZLAW_HOME/
├── zlaw.toml              # hub config + agent metadata (owned by ctl)
├── secrets.toml           # secrets (owned by ctl)
├── agent-assistant.toml   # agent config (owned by ctl)
├── agent-dev.toml         # agent config (owned by ctl)
└── agents/                # agent runtime data (owned by agent)
    ├── assistant/
    │   ├── SOUL.md
    │   ├── IDENTITY.md
    │   └── ...
    └── dev/
        ├── SOUL.md
        └── ...
```

The path is configurable via `zlaw.toml`, but convention is `$ZLAW_HOME/agent-{id}.toml`.

### zlaw.toml Agent Entry

```toml
[[agents]]
id = "assistant"
config = "agent-assistant.toml"  # path relative to $ZLAW_HOME, or absolute path
executor = "subprocess"
target = "local"
restart_policy = "on-failure"

[[agents.env_vars]]
from_secret = "MINIMAX_API_KEY"
name = "MINIMAX_API_KEY"
```

### Agent Config File ($ZLAW_HOME/agent-assistant.toml)

```toml
# LLM configuration
[llm]
backend = "anthropic"
model = "claude-sonnet-4-20250514"
client_config = { base_url = "https://api.anthropic.com/v1" }

# Adapter configuration
[[adapter]]
backend = "telegram"
client_config = { bot_token = "$TELEGRAM_BOT_TOKEN" }

# Identity (optional, or keep in agent home for hot-reload)
identity = "agents/assistant/IDENTITY.md"
soul = "agents/assistant/SOUL.md"

# Skills
skills = ["skill-read", "skill-write"]
```

Note: No `$VAR_NAME` references for secrets. Secrets are injected by ctl at spawn time based on `env_vars` mapping in `zlaw.toml`.

### Executor Interface

```go
type AgentConfig struct {
    ID            string
    ConfigFile    string  // path relative to $ZLAW_HOME, or absolute
    Executor      string
    Target        string
    RestartPolicy string
    EnvVars       []EnvVarMapping  // from zlaw.toml [[agents.env_vars]]
}

type Executor interface {
    Start(ctx context.Context, cfg AgentConfig) error
    // ...
}
```

### Subprocess Executor Behavior

For `executor = "subprocess"`:
- ctl passes config path via environment variable `ZLAW_AGENT_CONFIG` or `-c` flag
- Agent reads config from the specified path
- Agent home is for runtime data (SOUL.md, IDENTITY.md, sessions, etc.)

```bash
# ctl spawns agent with:
ZLAW_AGENT_CONFIG=$ZLAW_HOME/agent-assistant.toml zlaw-agent run
# or
zlaw-agent run -c $ZLAW_HOME/agent-assistant.toml
```

## Implementation

### Changes

1. **zlaw.toml** — add `config` field to agent entries (path to agent config file)
2. **Agent binary** — accept `-c` flag or `ZLAW_AGENT_CONFIG` env var for config file path
3. **ctl** — write agent configs to `$ZLAW_HOME/agent-{id}.toml` instead of `$ZLAW_HOME/agents/<id>/agent.toml`
4. **Interactive Setup** — update to write configs to new location
5. **Agent home** — no longer contains `agent.toml`

### Files Affected

| File | Change |
|------|--------|
| `internal/config/hub.go` | Add `Config` field to AgentConfig struct |
| `cmd/zlaw/setup/` | Write configs to `$ZLAW_HOME/agent-{id}.toml` |
| `cmd/zlaw-agent/` | Accept `-c` flag / `ZLAW_AGENT_CONFIG` env var |
| `internal/config/load.go` | Load config from file path |
| `internal/executor/subprocess.go` | Set `ZLAW_AGENT_CONFIG` env var when spawning |

### Doc Updates

- `agent_lifecycle.md` — remove `agent.toml` from agent home structure
- `agent_standalone.md` — update startup sequence
- `interactive_setup.md` — update config paths

## No Migration Path

No existing users. Fresh implementation only.

## See Also

- [agent_lifecycle.md](../docs/design/agent_lifecycle.md) — agent lifecycle
- [agent_standalone.md](../docs/design/agent_standalone.md) — standalone mode
- [interactive_setup.md](../docs/design/interactive_setup.md) — setup TUI
- [agent_secrets.md](../docs/design/agent_secrets.md) — secrets design