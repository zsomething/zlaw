# Interactive Setup (Onboarding)

## Goal

Provide an interactive TUI (`zlaw setup`) for configuring zlaw. Single menu shows all actions with state. Sub-screens handle individual configuration flows.

## Menu Structure

```
┌────────────────────────────────────────────┐
│  zlaw setup                                │
│                                             │
│  Bootstrap                                 │
│  ────────                                  │
│  ▶ Bootstrap Zlaw Home                     │
│    /home/user/.config/zlaw                  │
│    ✅ configured                           │
│                                             │
│  Agents                                    │
│  ──────                                    │
│  Agent: [assistant ▼]  (3)           │
│  ─────────────────────                      │
│  ● Configure LLM                          │
│    minimax                                 │
│    ⚠️ missing                              │
│  ● Configure adapter                      │
│    telegram                                │
│    ✅ configured                           │
│  ● Edit identity                          │
│    ✅ configured                           │
│  ● Edit soul                              │
│    ✅ configured                           │
│  ● Manage skills                          │
│    3 installed                             │
│                                             │
│  Global                                    │
│  ──────                                    │
│  ● Manage secrets                         │
│    2 secrets                               │
│  ● Summary                                │
│    view                                    │
│                                             │
│  ───────────────────────────────────────── │
│  [Q] Quit                                   │
└────────────────────────────────────────────┘
```

### Section Rules

| Section | Always Visible | Agent-specific Items |
|---------|----------------|----------------------|
| Bootstrap | Yes | No |
| Agents | Yes | No |
| Agent items | No (hidden) | Yes, require agent |
| Global | Yes | No |

### Item States

| State | Indicator | Meaning |
|-------|-----------|---------|
| Missing | ⚠️ | Required but not configured |
| Configured | ✅ | Set up and valid |
| Invalid | ❌ | Configured but broken (e.g., missing secret) |
| Installed/Count | number | Shows count (skills, secrets) |
| view | — | Opens read-only display |

### Visibility Rules

| Item | Show When |
|------|-----------|
| Agent section selector | At least one agent exists |
| Agent items | Agent selected |
| "No agents configured. Create one first." | No agents exist |

### Keyboard Navigation

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate menu |
| `Enter` | Select item |
| `Q` | Quit (sticky, always shown) |
| `B` | Back (sub-screens only) |

## Bootstrap Section

### Bootstrap Zlaw Home

**Purpose:** Create `$ZLAW_HOME/` and core files.

**States:**
- Not configured: `⚠️ not initialized`
- Configured: `✅ configured`

**Display:**
```
│  ▶ Bootstrap Zlaw Home                     │
│    /home/user/.config/zlaw                  │
│    ✅ configured                           │
```

**Flow (not configured):**
```
│  Create Zlaw Home?                         │
│                                             │
│  Path: /home/user/.config/zlaw             │
│                                             │
│  [Y] Create   [N] Cancel                   │
└────────────────────────────────────────────┘
```

**Flow (already configured):**
```
│  Zlaw Home already exists at:              │
│  /home/user/.config/zlaw                   │
│                                             │
│  [R] Re-create   [K] Keep   [N] Cancel    │
└────────────────────────────────────────────┘
```

**Creates:**
- `$ZLAW_HOME/zlaw.toml` (skeleton)
- `$ZLAW_HOME/secrets.toml` (empty, mode 0600)
- `$ZLAW_HOME/agents/` (directory)

## Agent Section

### Agent Selector

Dropdown to select which agent to configure. Shows all agents from `zlaw.toml`.

```
│  Agent: [assistant ▼]  (3)           │
```

When no agents exist:
```
│  Agents                                    │
│  ──────                                    │
│  ● No agents configured. Create one first. │
│                                             │
│  Global                                    │
```

Selecting "Create one first" opens agent creation flow.

### Create Agent

**Flow:**
```
│  Create Agent                              │
│                                             │
│  Agent ID: _                                │
│  > lowercase, alphanumeric + dash          │
│                                             │
│  Executor: [subprocess ▼]                   │
│  > [subprocess] [systemd] [docker]          │
│                                             │
│  Target: [local ▼]                          │
│  > [local] [ssh]                           │
│                                             │
│  Restart policy: [on-failure ▼]             │
│  > [always] [on-failure] [never]           │
│                                             │
│  [C] Create   [B] Back                      │
└────────────────────────────────────────────┘
```

**On create:**
1. Create `$ZLAW_HOME/agents/<id>/` directory
2. Create `$ZLAW_HOME/agents/<id>/agent.toml`
3. Create `$ZLAW_HOME/agents/<id>/SOUL.md`
4. Create `$ZLAW_HOME/agents/<id>/IDENTITY.md`
5. Create `$ZLAW_HOME/agents/<id>/skills/` directory
6. Add agent entry to `$ZLAW_HOME/zlaw.toml`

### Delete Agent

Available from agent selector dropdown or a delete option.

## Agent Items

All agent items show: label, current value/status, state indicator.

### Configure LLM

**States:**
- Missing: `⚠️ missing`
- Configured: `✅ configured` + backend name

**Flow:**
```
│  Configure LLM — assistant                 │
│                                             │
│  Select LLM preset:                        │
│  ─────────────────────                      │
│  1. minimax     — MiniMax API (Global)    │
│  2. minimax-cn  — MiniMax API (China)     │
│  3. anthropic   — Anthropic Claude        │
│  4. openai      — OpenAI GPT               │
│                                             │
│  [B] Back                                   │
└────────────────────────────────────────────┘
```

**After preset selection — Secret Setup:**
```
│  LLM: minimax (anthropic backend)          │
│                                             │
│  This preset requires:                      │
│  • api_key — Env var: MINIMAX_API_KEY      │
│                                             │
│  Secret: [Create new ▼]                     │
│  > [Create new] [Use existing]             │
│                                             │
│  Secret name: [MINIMAX_API_KEY]            │
│                                             │
│  [C] Configure   [B] Back                    │
└────────────────────────────────────────────┘
```

**Display after configured:**
```
│  ● Configure LLM                          │
│    minimax                                 │
│    ✅ configured                           │
```

### Configure Adapter

**States:**
- Missing: `⚠️ no adapter`
- Configured: `✅ telegram` (or other)

**Flow:**
```
│  Configure Adapter — assistant             │
│                                             │
│  Select adapter:                           │
│  ─────────────────                         │
│  1. telegram  — Telegram Bot API          │
│  2. slack     — Slack webhook             │
│  3. None     — NATS only (no adapter)     │
│                                             │
│  [B] Back                                   │
└────────────────────────────────────────────┘
```

**After selection — Secret Setup (if needed):**
```
│  Adapter: telegram                         │
│                                             │
│  This adapter requires:                    │
│  • bot_token — Env var: TELEGRAM_BOT_TOKEN │
│                                             │
│  Secret: [Create new ▼]                    │
│                                             │
│  [C] Configure   [B] Back                   │
└────────────────────────────────────────────┘
```

### Edit Identity

Opens `IDENTITY.md` in `$EDITOR`.

**Display:**
```
│  ● Edit identity                          │
│    IDENTITY.md                            │
│    ✅ configured                           │
```

### Edit Soul

Opens `SOUL.md` in `$EDITOR`.

**Display:**
```
│  ● Edit soul                              │
│    SOUL.md                                │
│    ✅ configured                           │
```

### Manage Skills

Shows list of skills, allows adding/removing.

**Display:**
```
│  ● Manage skills                          │
│    skills/                                │
│    3 installed                            │
│                                             │
│  skills:                                   │
│  ──────                                    │
│  1. weather                                │
│  2. calendar                               │
│  3. slack-notify                           │
│                                             │
│  [A] Add skill   [R] Remove   [B] Back    │
└────────────────────────────────────────────┘
```

## Global Section

### Manage Secrets

View, add, remove secrets in `secrets.toml`.

```
│  Manage Secrets                           │
│                                             │
│  secrets.toml                              │
│  2 secrets                                 │
│  ──────────                                │
│  1. MINIMAX_API_KEY          set           │
│  2. TELEGRAM_BOT_TOKEN      set           │
│                                             │
│  [A] Add secret   [R] Remove   [B] Back  │
└────────────────────────────────────────────┘
```

### Summary

Read-only view of current configuration.

```
│  Configuration Summary                    │
│                                             │
│  Bootstrap                                 │
│  ──────────                                │
│  zlaw_home:  /home/user/.config/zlaw       │
│  status:     ✅ configured                │
│                                             │
│  Agent: assistant                         │
│  ───────────────                           │
│  LLM:       minimax     ✅ configured      │
│  Adapter:   telegram    ✅ configured     │
│  Identity:  IDENTITY.md  ✅ configured    │
│  Soul:      SOUL.md      ✅ configured    │
│  Skills:    3                             │
│                                             │
│  Secrets:   2 configured                  │
│                                             │
│  Next steps:                               │
│  $ zlaw ctl start                          │
│    → Start hub + agents                    │
│                                             │
│  [B] Back                                   │
└────────────────────────────────────────────┘
```

## Implementation Notes

### TUI Framework

**Dependency:** `github.com/charmbracelet/bubbletea`

Bubble Tea model:
- Each screen (main menu, LLM config, adapter config, etc.) is a separate `tea.Model`
- Screens return to main menu via `tea.Quit` + state update
- Global state (selected agent, config cache) passed via initial model

### Project Structure

```
cmd/zlaw/setup/
├── main.go           # setup command entry
├── menu.go           # main menu model
├── bootstrap.go       # bootstrap screen
├── agent.go          # agent creation screen
├── agent_list.go     # (future) multi-agent selector
├── llm.go            # LLM configuration screen
├── adapter.go        # adapter configuration screen
├── secrets.go        # secrets management screen
├── skills.go         # skills management screen
├── summary.go        # summary screen
├── state.go          # shared state (selected agent, config cache)
└── styles.go         # Bubble Tea styles
```

### State Management

```go
type State struct {
    Home         string           // ZLAW_HOME path
    Config       *config.HubConfig
    Secrets      secrets.Store
    SelectedAgent string         // agent ID or ""
}

type Model struct {
    state  State
    screen ScreenType
    // ... screen-specific fields
}
```

### Sub-screen Navigation

Each sub-screen returns `tea.Model` with updated state. Main loop dispatches to appropriate screen:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch m.screen {
    case ScreenMainMenu:
        return m.updateMainMenu(msg)
    case ScreenLLM:
        return m.updateLLM(msg)
    case ScreenAdapter:
        return m.updateAdapter(msg)
    // ...
    }
}
```

## CLI Reference

```bash
# Interactive setup wizard
zlaw setup
```

No command-line flags for individual steps — all navigation is via menu.

## Open Questions

1. **Model selection flow:** After LLM config, prompt for model via API call. Allow proceed with warning if fetch fails.

## Resolved Design Decisions

- **Non-interactive mode:** Not needed. Automation uses existing `zlaw init --agent`, `zlaw auth add`, etc.
- **cli adapter:** Not a selectable preset; use `zlaw agent run` directly.
- **Menu vs wizard flow:** Menu-based navigation, not linear wizard.
- **Item visibility:** Agent items hidden when no agents; shown (possibly disabled) when agent selected.
- **Sub-screen replacement:** Sub-screens replace the menu, not inline.

## See Also

- [llm_presets.md](./llm_presets.md) — LLM preset pattern
- [agent_secrets.md](./agent_secrets.md) — secrets injection design
- [channel_adapter.md](./channel_adapter.md) — adapter presets
- [agent_lifecycle.md](./agent_lifecycle.md) — agent home structure