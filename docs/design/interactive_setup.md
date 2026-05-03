# Interactive Setup (Onboarding)

## Goal

Interactive TUI (`zlaw setup`) for configuring zlaw. Wires to `internal/config/` for actual operations.

## Key Design Rules

1. **Use bubbles + lipgloss.** Never invent custom components.
2. **Full-screen window, web form style.** Centered content, clean padding.
3. **No inline shortcuts.** Footer help bar only.
4. **State drives visibility.** Menu items appear/disappear based on config state.

## Framework

```
tea.Model → Update(msg tea.Msg) → View() string
```

Bubbles used:
- `textinput` — form fields
- `spinner` — loading states
- `viewport` — scrollable content
- `table` — tabular data

Lipgloss for styling and layout composition.

## Layout

```
┌─────────────────────────────────────────────────────────────┐
│  zlaw setup                                               │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     Section Title                                           │
│     ───────────                                             │
│       Item Label              [status]                     │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit     │
└─────────────────────────────────────────────────────────────┘
```

## Colors

| Element | Hex | Usage |
|---------|-----|-------|
| Accent | `#00aaee` | Selected item, highlights |
| Dim | `#666666` | Secondary info, status |
| Success | `#00cc66` | configured |
| Warning | `#ffaa00` | missing |
| Error | `#ff4444` | invalid |

## Navigation

- `pushScreen(s)` — push to stack, navigate to screen
- `popScreen()` — pop from stack, return to previous
- Screen-specific `Update`/`View` methods on the model

## Menu Item Specifications

### Bootstrap Section

#### Bootstrap Zlaw Home

**Display (not bootstrapped / incomplete):**

```
┌─────────────────────────────────────────────────────────────┐
│  Bootstrap Zlaw Home                                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     Target path:                                             │
│     /home/user/.config/zlaw                                 │
│     From ZLAW_HOME env var                                  │
│                                                              │
│     [Create]   [Cancel]                                     │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [↑↓] Navigate   [Enter] Select   [←] Back                 │
└─────────────────────────────────────────────────────────────┘
```

**Display (bootstrapped):**

```
┌─────────────────────────────────────────────────────────────┐
│  Bootstrap Zlaw Home                                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     Target path:                                             │
│     /home/user/.config/zlaw                                 │
│                                                              │
│     ✓ Already configured                                    │
│                                                              │
│     [Reset]   [Keep]   [Cancel]                            │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [↑↓] Navigate   [Enter] Select   [←] Back                 │
└─────────────────────────────────────────────────────────────┘
```

**Confirmation Dialog (for Reset):**

```
┌─────────────────────────────────────────────────────────────┐
│  ⚠️ Reset Bootstrap?                                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     This will recreate the zlaw home structure.            │
│     Existing files will be preserved where possible.       │
│                                                              │
│     [Yes, Reset]   [No, Cancel]                            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Properties:**

| Property | Value |
|----------|-------|
| **Label** | `Bootstrap Zlaw Home` |
| **Visibility** | Always |
| **Path** | Absolute path from `$ZLAW_HOME`, with note if env var is set |
| **Status: Not Bootstrapped** | `⚠️ not initialized` — `zlaw.toml` does not exist |
| **Status: Bootstrapped** | `✅ configured` |
| **Status: Incomplete** | `⚠️ incomplete setup` — dir exists but `zlaw.toml` missing/malformed |

**Actions:**

| Action | Condition | Behavior |
|--------|-----------|----------|
| `Create` | Not bootstrapped / Incomplete | `CreateZlawHome()` → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
| `Reset` | Bootstrapped | Show confirmation dialog → `CreateZlawHome(Force: true)` → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
| `Keep` | Bootstrapped | `popScreen()` |
| `Cancel` | Always | `popScreen()` |

**States:**

| State | Display |
|-------|---------|
| **Success** | `✓ Done` — stays until user navigates |
| **Error** | `⚠️ <error message>` inline — buttons remain active for retry |
| **Incomplete** | Directory exists but `zlaw.toml` missing/malformed — offer `Create` to fix |

---

### Agents Section

**Visibility:** `IsBootstrapped == true`

#### Create Agent

**Display (menu item):** `Create Agent`

**Create Agent Screen:**

```
┌─────────────────────────────────────────────────────────────┐
│  Create Agent                                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     Agent ID                                                │
│     ┌─────────────────────────────────┐                    │
│     │ _                               │                    │
│     └─────────────────────────────────┘                    │
│     lowercase, alphanumeric, dash only                      │
│                                                              │
│     Executor                                                │
│     [subprocess] (local only)                              │
│                                                              │
│     [Create Agent]   [Cancel]                               │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [↑↓] Navigate   [Tab] Next   [Enter] Select   [←] Back   │
└─────────────────────────────────────────────────────────────┘
```

**Form Fields:**

| Field | Type | Validation |
|-------|------|------------|
| Agent ID | textinput | Required, lowercase alphanumeric + dash, no leading/trailing dash, unique |
| Executor | selector | `subprocess` only (disabled, only local option available) |

**Actions:**

| Action | Condition | Behavior |
|--------|-----------|----------|
| `Create Agent` | Form valid | `CreateAgent()` → Success: `✓ Agent 'id' created` + auto-select agent + `popScreen()` / Error: stay, show error inline |
| `Cancel` | Always | `popScreen()` |

**States:**

| State | Display |
|-------|---------|
| **Success** | `✓ Agent 'id' created` — stays until user navigates |
| **Error: Invalid ID** | `⚠️ invalid format` inline |
| **Error: Duplicate ID** | `⚠️ agent 'id' already exists` inline |
| **Error: Other** | `⚠️ <error message>` inline |

---

#### Select Agent

**Display (menu item):** `Select Agent: <name>` or `Select Agent: (none)`

**Select Agent Screen:**

```
┌─────────────────────────────────────────────────────────────┐
│  Select Agent                                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│     ▸ assistant                                            │
│       gpt4                                                  │
│       dev-agent                                            │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit     │
└─────────────────────────────────────────────────────────────┘
```

**Properties:**

| Property | Value |
|----------|-------|
| **Label** | `Select Agent: <name>` or `Select Agent: (none)` |
| **Visibility** | When bootstrapped AND agents exist |
| **Status** | Disabled (dimmed) when no agents |

<b>Actions:</b>

 Action | Behavior |
--------|----------|
 `Select` | Set `SelectedAgent` to chosen ID, `popScreen()` |

<b>Notes:</b>
- Single-select only
- No delete, no reorder

---

#### Configure LLM

| Property | Value |
|----------|-------|
| **Label** | `Configure LLM` |
| **Visibility** | `SelectedAgent != ""` |
| **Status: Missing** | `⚠️ missing` — no `[llm]` in `$ZLAW_HOME/agent-{id}.toml` |
| **Status: Configured** | `✅ configured` — valid `[llm]` section in $ZLAW_HOME/agent-{id}.toml |
| **Status: Invalid** | `❌ invalid` — config malformed |


<b>**LLMConfig Screen (preset selection):</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Configure LLM — assistant                                 │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     Select LLM preset:                                       ||
|                                                              ||
|     ▸ minimax          (MINIMAX_API_KEY)                   ||
|       anthropic        (ANTHROPIC_API_KEY)                  ||
|       openai           (OPENAI_API_KEY)                     ||
|       openrouter       (OPENROUTER_API_KEY)                ||
|                                                              ||
|  [↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit     │
└─────────────────────────────────────────────────────────────┘
```

<b>**LLMConfig Screen (secret configuration):</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Configure LLM — assistant                                 │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     MINIMAX_API_KEY is required                             │
|                                                              ||
|     Use existing secret:                                    │
|     [MINIMAX_API_KEY (prod) ▼]                             │
|     [Use Secret]                                            │
|                                                              ||
|     ─────────────────────────────────────────────────────  │
|                                                              ||
|     Or create new secret:                                   │
|                                                              ||
|     Key                                                     │
|     ┌─────────────────────────────────┐                    │
|     │ MINIMAX_API_KEY                  │                    │
|     └─────────────────────────────────┘                    │
|                                                              ||
|     Value                                                   │
|     ┌─────────────────────────────────┐                    │
|     │ _                               │                    │
|     └─────────────────────────────────┘                    │
|                                                              ||
|     [Create Secret]                                         │
|                                                              ||
|  [↑↓] Navigate   [Tab] Next   [←] Back                      │
└─────────────────────────────────────────────────────────────┘
```

<b>**Properties:</b>

 Property | Value |
----------|-------|
 **Presets** | minimax, anthropic, openai, openrouter |
 **Required Env Vars** | Per preset (shown after selection) |
 **Secret Selection** | Dropdown (pre-selected if matching name exists) OR create new |
 **Mapping** | Stored in `zlaw.toml` under agent's `env_vars` |

<b>**Actions:</b>

 Action | Behavior |
--------|----------|
 `Use Secret` | Add mapping to `zlaw.toml`. Write `$ZLAW_HOME/agent-{id}.toml` with preset. → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
 `Create Secret` | Save new secret to `secrets.toml`. Add mapping to `zlaw.toml`. Write `$ZLAW_HOME/agent-{id}.toml` with preset. → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
 `Cancel` | `popScreen()` |

<b>**Notes:</b>

 - Dropdown pre-selected if existing secret matches required env var name
 - New secret key pre-filled with required env var name, editable
 - Value field not masked
 - Multiple required env vars shown in one form
 - Mapping stored as `{ name = "ENV_VAR", from_secret = "SECRET_NAME" }` in `zlaw.toml`

---

#### Configure Adapter

<b>**AdapterConfig Screen (preset selection):</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Configure Adapter — assistant                              │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     Select adapter preset:                                   │
|                                                              ||
|     ▸ telegram         (TELEGRAM_BOT_TOKEN)                 │
|                                                              ||
|  [↑↓] Navigate   [Enter] Select   [←] Back   [Q] Quit     │
└─────────────────────────────────────────────────────────────┘
```

<b>**AdapterConfig Screen (secret configuration):</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Configure Adapter — assistant                              │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     TELEGRAM_BOT_TOKEN is required                          │
|                                                              ||
|     Use existing secret:                                    │
|     [TELEGRAM_BOT_TOKEN (prod) ▼]                           │
|     [Use Secret]                                            │
|                                                              ||
|     ─────────────────────────────────────────────────────  │
|                                                              ||
|     Or create new secret:                                   │
|                                                              ||
|     Key                                                     │
|     ┌─────────────────────────────────┐                    │
|     │ TELEGRAM_BOT_TOKEN               │                    │
|     └─────────────────────────────────┘                    │
|                                                              │
|     Value                                                   │
|     ┌─────────────────────────────────┐                    │
|     │ _                               │                    │
|     └─────────────────────────────────┘                    │
|                                                              ||
|     [Create Secret]                                         │
|                                                              │
|  [↑↓] Navigate   [Tab] Next   [←] Back                      │
└─────────────────────────────────────────────────────────────┘
```

<b>**Properties:</b>

 Property | Value |
----------|-------|
 **Presets** | telegram |
 **Required Env Vars** | `TELEGRAM_BOT_TOKEN` |
 **Secret Selection** | Dropdown OR create new |
 **Mapping** | Stored in `zlaw.toml` under agent's `env_vars` |

<b>**Actions:</b>

 Action | Behavior |
--------|----------|
 `Use Secret` | Add mapping to `zlaw.toml`. Write `$ZLAW_HOME/agent-{id}.toml` with adapter. → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
 `Create Secret` | Save to `secrets.toml`. Add mapping to `zlaw.toml`. Write `agent.toml`. → Success: `✓ Done` + `popScreen()` / Error: stay, show error inline |
 `Cancel` | `popScreen()` |

<b>**Notes:</b>

 - Same secret UI pattern as LLM configuration
 - Only telegram adapter available for now
 - Mapping stored as `{ name = "TELEGRAM_BOT_TOKEN", from_secret = "SECRET_NAME" }` in `zlaw.toml`

---
#### Edit Identity

| Property | Value |
|----------|-------|
| **Label** | `Edit Identity` |
| **Visibility** | `SelectedAgent != ""` |
| **Status: Missing** | `⚠️ missing` — IDENTITY.md does not exist |
| **Status: Configured** | `✅ configured` — IDENTITY.md exists |

**Actions:** `[Select]` → open `$ZLAW_HOME/agents/<id>/IDENTITY.md` in `$EDITOR`

---

#### Edit Soul

| Property | Value |
|----------|-------|
| **Label** | `Edit Soul` |
| **Visibility** | `SelectedAgent != ""` |
| **Status: Missing** | `⚠️ missing` |
| **Status: Configured** | `✅ configured` |

**Actions:** `[Select]` → open `$ZLAW_HOME/agents/<id>/SOUL.md` in `$EDITOR`

---

#### Manage Skills

| Property | Value |
|----------|-------|
| **Label** | `Manage Skills` |
| **Visibility** | `SelectedAgent != ""` |
| **Status** | `N skills` badge |

**Actions:** `[Select]` → `pushScreen(ScreenSkills)`

---

### Global Section

**Visibility:** Always

#### Manage Secrets

<b>**Secrets Screen (main view):</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Manage Secrets                                            │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     MINIMAX_API_KEY                                         ||
|     ANTHROPIC_API_KEY                                       ||
|     TELEGRAM_BOT_TOKEN                                      ||
|                                                              ||
|     [+ Add Secret]                                         │
|                                                              ||
|  [↑↓] Navigate   [Enter] Select   [+ Add]   [←] Back       │
└─────────────────────────────────────────────────────────────┘
```

<b>**Add/Edit Secret Screen:</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Add Secret                    Edit Secret                  │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     Key                                                     │
|     ┌─────────────────────────────────┐                    │
|     │ MINIMAX_API_KEY                  │                    │
|     └─────────────────────────────────┘                    │
|                                                              ||
|     Value                                                   │
|     ┌─────────────────────────────────┐                    │
|     │ _                               │                    │
|     └─────────────────────────────────┘                    │
|                                                              ||
|     [Save]   [Cancel]                                      │
|                                                              ||
|  [↑] Navigate   [Tab] Next   [Enter] Select   [←] Back     │
└─────────────────────────────────────────────────────────────┘
```

<b>**Delete Confirmation:</b>

```
┌─────────────────────────────────────────────────────────────┐
|  ⚠️ Delete Secret?                                         │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     Delete MINIMAX_API_KEY?                                 │
|     This cannot be undone.                                  │
|                                                              ||
|     [Yes, Delete]   [No, Cancel]                           │
|                                                              ||
└─────────────────────────────────────────────────────────────┘
```

<b>**Properties:</b>

 Property | Value |
----------|-------|
 **Location** | `secrets.toml` at `$ZLAW_HOME/` |
 **Display** | Secret names only (from TOML keys) |
 **Operations** | List, Add, Edit, Remove |

<b>**Actions:</b>

 Action | Behavior |
--------|----------|
 `Add Secret` | Open add form → `Save` writes to `secrets.toml` |
 `Edit Secret` | Open edit form with pre-filled key → `Save` updates value |
 `Delete` | Confirmation dialog → removes from `secrets.toml` |
 `Cancel` | Return to previous screen |

<b>**Notes:</b>

 - Edit shows key (read-only), value field empty (no current value for security)
 - Delete requires confirmation dialog
 - Value field not masked

---

#### Summary

<b>**Summary Screen:</b>

```
┌─────────────────────────────────────────────────────────────┐
|  Configuration Summary                                     │
|├─────────────────────────────────────────────────────────────┤
|                                                              ||
|     BOOTSTRAP                                               ||
|     Path:      ~/.config/zlaw                               ||
|     Status:   configured                                  ||
|                                                              ||
|     AGENTS                                                   ||
|     ▸ assistant                                            │
|       gpt4                                                  │
|                                                              ||
|     AGENT: assistant                                        ||
|     LLM:      configured                                   ||
|     Adapter:  missing                                      │
|     Identity: configured                                   │
|     Soul:      missing                                    │
|     Skills:   3                                            ||
|                                                              ||
|     SECRETS                                                 │
|     Count: 2                                                │
|                                                              ||
|  [←] Back   [Q] Quit                                      ||
└─────────────────────────────────────────────────────────────┘
```

<b>**Properties:</b>

 Property | Value |
----------|-------|
 **Type** | Read-only |
 **Content** | Bootstrap status, agents list, per-agent config status, secrets count |

<b>**Actions:</b>

 Action | Behavior |
--------|----------|
 `Back` | `popScreen()` |

---

## Keyboard Navigation

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate list |
| `Enter` | Select |
| `←` or `Esc` | Back |
| `Tab` | Next field (forms) |
| `Q` / `Ctrl+C` | Quit |

## Project Structure

```
cmd/zlaw/setup/
├── main.go      # Entry point, LoadAppState()
├── model.go     # Root Model, screen routing
├── types.go     # ScreenType enum
├── state.go     # AppState, LoadAppState()
├── styles.go    # Lipgloss styles
└── screens.go   # Screen view/update implementations
```

## AppState

```go
type ItemState int

const (
    StateMissing ItemState = iota
    StateConfigured
    StateInvalid
    StateView  // action-only
)

type AppState struct {
    HomePath       string
    IsBootstrapped bool
    SelectedAgent  string
    Agents         []string
    LLMStatus      ItemState
    AdapterStatus  ItemState
    IdentityStatus ItemState
    SoulStatus     ItemState
    SecretsCount   int
    SkillsCount    int
}
```

## Status Detection

| Item | Detection |
|------|-----------|
| `IsBootstrapped` | `os.Stat(HomePath + "/zlaw.toml")` succeeds |
| `LLMStatus` | Check `agent.toml` for `[llm]` section |
| `AdapterStatus` | Check `agent.toml` for `[adapter]` section |
| `IdentityStatus` | `os.Stat(IDENTITY.md)` succeeds |
| `SoulStatus` | `os.Stat(SOUL.md)` succeeds |
| `SecretsCount` | Entries in `secrets.toml` |
| `SkillsCount` | `.md` files in `skills/` |

## See Also

- [llm_presets.md](./llm_presets.md)
- [agent_secrets.md](./agent_secrets.md)
- [channel_adapter.md](./channel_adapter.md)
- Bubble Tea docs: https://github.com/charmbracelet/bubbletea