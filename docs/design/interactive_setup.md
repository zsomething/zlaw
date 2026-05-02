# Interactive Setup (Onboarding)

## Goal

Provide an interactive TUI (`zlaw setup`) for configuring zlaw. Single menu shows all actions with state. Sub-screens handle individual configuration flows.

## Menu Structure

```
┌────────────────────────────────────────────┐
│  zlaw setup                                │
│                                             │
│  Bootstrap                                 │
│  ─────────────────────                      │
│  ▶ Bootstrap Zlaw Home                     │
│    configured                             │
│                                             │
│  Agents                                    │
│  ─────────────────────                      │
│  ▶ Create Agent                            │
│  ▶ Select Agent: assistant (3)            │
│    Configure LLM                          │
│    Configure adapter                        │
│    Edit identity                            │
│    Edit soul                                │
│    Manage skills                            │
│                                             │
│  Global                                    │
│  ─────────────────────                      │
│  ▶ Manage secrets  0 secrets               │
│  ▶ Summary view                            │
│                                             │
│  ───────────────────────────────────────── │
│  [↑↓] Navigate  [Enter] Select  [Q] Quit     │
└────────────────────────────────────────────┘
```

**When no agents exist:**
```
│  Agents                                    │
│  ─────────────────────                      │
│  ▶ Create Agent                            │
│    Select Agent: (no agents)  ← disabled   │
│                                             │

### Section Rules

| Section | Always Visible | Notes |
|---------|----------------|-------|
| Bootstrap | Yes | Always |
| Agents | Yes (if bootstrapped) | |
| Create Agent | Yes | Always when bootstrapped |
| Select Agent | Yes | Disabled if no agents |
| Agent items | Conditional | Shown when agent selected |
| Global | Yes | Always |

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
| Create Agent | Home bootstrapped |
| Select Agent | Home bootstrapped (disabled if no agents) |
| Agent config items | An agent is selected |
| Agent selector | At least one agent exists |

### Keyboard Navigation

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate menu |
| `Enter` | Select item / confirm action |
| `←` or `h` | Back (previous screen) |
| `Esc` | Back (same as ←) |
| `Tab` | Next field (in forms) |
| `Q` | Quit (sticky, always shown) |

### Text Input Handling

When typing in input fields (e.g., Agent ID, Secret name):
- Letter keys are consumed as text input, **not** as shortcuts
- Only `Backspace` and navigation keys interrupt input
- This prevents letters like 'b' being caught as "Back" shortcut

### Visual Design

**Window-style layout:**
- Blue title bar header (`zlaw setup`, `Bootstrap`, etc.)
- `▶` prefix for selected items (no numbers)
- `Selected` style: blue background with white text
- `ItemDim` style for inactive/secondary text

**No inline shortcuts:** All keyboard actions shown only in footer help bar. Avoids conflicts with text input fields.

## Shared Config Management

All setup and configuration operations are implemented in `internal/config/` to enable reuse across all entry points:

| Entry Point | Uses |
|-------------|------|
| `zlaw setup` (interactive TUI) | `internal/config/` |
| `zlaw init` (non-interactive CLI) | `internal/config/` |
| `zlaw hub` (auto-bootstrap on startup) | `internal/config/` |

### Packages

```
internal/config/
├── hub.go       # HubConfig, AgentEntry, zlaw.toml load/save
├── config.go    # AgentConfig (LLM/adapter settings per agent)
├── bootstrap.go # BootstrapConfig, SetupAgentConfig (setup operations)
```

### BootstrapConfig

Creates `$ZLAW_HOME/` structure (zlaw.toml, secrets.toml, agents/):

```go
cfg := config.BootstrapConfig{
    Home:     "~/.config/zlaw",
    Force:    false, // error if exists
}
if err := cfg.CreateZlawHome(); err != nil {
    // handle error
}
```

### SetupAgentConfig

Creates agent directory structure (SOUL.md, IDENTITY.md, workspace/, skills/):

```go
cfg := config.DefaultSetupAgentConfig("assistant")
cfg.Force = false
if err := cfg.CreateAgent(); err != nil {
    // handle error
}
```

### Principles

1. **Shared logic**: All config operations live in `internal/config/`, never in `cmd/`
2. **Composable**: Config structs have sensible defaults, can override fields
3. **Idempotent**: Operations check existence and support `Force` flag
4. **Error messaging**: Errors include context for user-friendly display

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
│  Setup                                    │
│                                             │
│  Agent ID: _                                │
│    lowercase, alphanumeric + dash          │
│                                             │
│  Executor: [subprocess ▼]                   │
│                                             │
│  Target: [local ▼]                          │
│                                             │
│  Restart policy: [on-failure ▼]             │
│                                             │
│  [↑↓] Navigate  [Enter] Create  [←] Back       │
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
- Missing: `missing`
- Configured: `configured` + backend name

**Flow:**
```
│  Configure LLM — assistant                 │
│                                             │
│  Select LLM preset:                        │
│  ─────────────────────                      │
│  ▶ minimax     —  MiniMax API (Global)     │
│    anthropic   —  Anthropic Claude          │
│    openai     —  OpenAI GPT                │
│                                             │
│  [↑↓] Navigate  [Enter] Select  [←] Back       │
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
- Missing: `missing`
- Configured: `configured` + adapter name

**Flow:**
```
│  Configure Adapter — assistant             │
│                                             │
│  Select adapter:                           │
│  ─────────────────                         │
│  ▶ telegram   —  Telegram Bot API          │
│    slack      —  Slack webhook             │
│    none       —  NATS only (no adapter)    │
│                                             │
│  [↑↓] Navigate  [Enter] Select  [←] Back       │
└────────────────────────────────────────────┘
```

**After selection — Secret Setup:**
```
│  Adapter: telegram                         │
│                                             │
│  This adapter requires:                    │
│  • bot_token — Env var: TELEGRAM_BOT_TOKEN │
│                                             │
│  Secret: [Create new ▼]                    │
│                                             │
│  [↑↓] Navigate  [Tab] Next field  [Enter] Done  [←] Back │
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
│  Manage Skills — assistant                 │
│                                             │
│  ▶ weather                                │
│    calendar                                │
│    slack-notify                            │
│                                             │
│  [↑↓] Navigate  [Enter] Add  [←] Back         │
└────────────────────────────────────────────┘
```

## Global Section

### Manage Secrets

View, add, remove secrets in `secrets.toml`.

```
│  Manage secrets                            │
│                                             │
│  ▶ MINIMAX_API_KEY                        │
│    TELEGRAM_BOT_TOKEN                     │
│                                             │
│  [↑↓] Navigate  [Enter] Add/Delete  [←] Back │
└────────────────────────────────────────────┘
```

### Summary

Read-only view of current configuration.

```
│  Configuration Summary                    │
│                                             │
│  Bootstrap                                 │
│  ──────────                                │
│  zlaw_home:  ~/.config/zlaw                │
│  status:     configured                   │
│                                             │
│  Agent: assistant                         │
│  ───────────────                           │
│  LLM:       minimax     configured         │
│  Adapter:   telegram    configured        │
│  Identity:  IDENTITY.md  configured        │
│  Soul:      SOUL.md      configured        │
│  Skills:    3                             │
│                                             │
│  Secrets:   2 configured                  │
│                                             │
│  [↑↓] Navigate  [←] Back                     │
└────────────────────────────────────────────┘
```

## Implementation Notes

### TUI Framework

**Dependencies:**
- `github.com/charmbracelet/bubbletea` — main TUI framework
- `github.com/charmbracelet/bubbles` — pre-built components

**Use bubbles components, don't invent your own.** The bubbles library provides well-tested, accessible components:

| Component | Use |
|-----------|-----|
| `textinput` | Form fields (Agent ID, Secret name/value) |
| `textarea` | Multi-line editors |
| `spinner` | Loading states |
| `viewport` | Scrollable content |
| `progressbar` | Long operations |
| `keymap` | Declarative key bindings |
| `table` | Tabular data display |
| `ticker` | Periodic updates |
| `viewport` | Scrollable content |

**Why bubbles?**
- Handles input correctly (e.g., letter keys don't trigger shortcuts while typing)
- Accessible by default
- Consistent behavior across screens
- Battle-tested in production

**Anti-pattern to avoid:**
```go
// DON'T: Build custom text input handling
if msg.Runes[0] >= 'a' && msg.Runes[0] <= 'z' {
    m.input += string(msg.Runes[0])
}

// DO: Use textinput component
tea.Model with bubbles/textinput.Model
```

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