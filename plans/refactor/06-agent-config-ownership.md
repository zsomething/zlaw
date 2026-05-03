# 06: Agent Config Ownership Refactoring

## Goal

Move agent configuration (`llm`, `adapter`, etc.) out of per-agent `agent.toml` files in `$ZLAW_HOME/agents/<id>/` to be owned and managed by ctl, similar to how `zlaw.toml` and `secrets.toml` are owned.

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
1. **Sandboxed isolation**: Agent receives config via process arguments (`-c agent.toml`) or env var, not filesystem access
2. **Centralized management**: All configs under ctl control
3. **Clean separation**: Agent is just a process, doesn't need filesystem knowledge
4. **Flexibility**: For local subprocess executor, agent CAN still access its config file (via `-c` path). For sandboxed executors, config is passed differently.

## Design

### New Config Structure

```
$ZLAW_HOME/
├── zlaw.toml              # hub config + agent metadata (owned by ctl)
├── secrets.toml           # secrets (owned by ctl)
└── agent-configs/          # agent configuration files (owned by ctl)
    ├── agent-assistant.toml
    ├── agent-dev-agent.toml
    └── agent-master.toml
```

### zlaw.toml Agent Entry

```toml
[[agents]]
id = "assistant"
config_file = "agent-configs/agent-assistant.toml"
executor = "subprocess"
target = "local"
restart_policy = "on-failure"

[[agents.env_vars]]
from_secret = "MINIMAX_API_KEY"
name = "MINIMAX_API_KEY"
```

### Agent Config File (agent-configs/agent-assistant.toml)

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
identity = "IDENTITY.md"
soul = "SOUL.md"

# Skills
skills = ["skill-read", "skill-write"]
```

Note: No `$VAR_NAME` references for secrets. Secrets are injected by ctl at spawn time based on `env_vars` mapping in `zlaw.toml`.

### Executor Interface Change

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
- ctl passes `-c <config_file_path>` to agent binary
- Agent reads config from CLI arg, not from `$ZLAW_AGENT_HOME/agent.toml`
- For backwards compatibility during transition, if no `-c` provided, fall back to `$ZLAW_AGENT_HOME/agent.toml`

```bash
# ctl spawns agent with:
zlaw-agent run -c $ZLAW_HOME/agent-configs/agent-assistant.toml
```

### Future: Sandboxed Executors

For sandboxed executors (containers, etc.):
- Config file is passed as argument but may not be mounted into sandbox
- Agent receives config content via environment variable or stdin
- Design TBD per executor type

## Implementation Phases

### Phase 1: Config File Separation
- Create `agent-configs/` directory in `$ZLAW_HOME`
- Add `config_file` field to agent entries in `zlaw.toml`
- Modify `CreateAgent()` to write config to `agent-configs/` instead of agent home
- Modify agent binary to accept `-c` flag
- Update ctl to pass `-c` flag when spawning

### Phase 2: Update Interactive Setup
- Modify Configure LLM/Adapter to write to `agent-configs/` directory
- Remove write access to `agent home/agent.toml` for config sections
- Keep `SOUL.md`, `IDENTITY.md` in agent home (user-editable content)

### Phase 3: Remove Legacy Support
- Remove fallback to `$ZLAW_AGENT_HOME/agent.toml`
- Clean up agent home directory structure (remove `agent.toml` from agent homes)

## Files Affected

### Config Management
- `internal/config/hub.go` — add `config_file` to AgentConfig struct
- `internal/config/config.go` — add `AgentConfigDir()` function
- `cmd/zlaw/setup/` — update to write configs to new location

### Agent Binary
- `cmd/zlaw-agent/` — accept `-c` flag for config file path
- `internal/config/load.go` — load config from file path

### Executor
- `internal/executor/subprocess.go` — pass `-c` flag when spawning
- `internal/executor/` — interface updates (if needed)

### Documentation
- `docs/design/agent_lifecycle.md` — update agent home structure
- `docs/design/agent_standalone.md` — update startup sequence
- `docs/design/interactive_setup.md` — update config paths

## Migration Path

1. New agents: config in `agent-configs/`, no `agent.toml` in agent home
2. Existing agents: continue reading `agent.toml` from agent home (backwards compat)
3. Migration script: move existing `agent.toml` to `agent-configs/`, update `zlaw.toml`

## Open Questions

1. Should agent home still exist? For what? (SOUL.md, IDENTITY.md, sessions, memories, workspace)
2. How does migration work for existing setups?
3. For sandboxed executors, how is config passed (env var, stdin, mounted file)?

## See Also

- [agent_lifecycle.md](../docs/design/agent_lifecycle.md) — agent lifecycle
- [agent_standalone.md](../docs/design/agent_standalone.md) — standalone mode
- [interactive_setup.md](../docs/design/interactive_setup.md) — setup TUI
- [agent_secrets.md](../docs/design/agent_secrets.md) — secrets design