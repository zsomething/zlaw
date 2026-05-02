# Implementation Plan: Interactive Setup Wizard

## Overview

Implement `zlaw setup` — a menu-based TUI for configuring zlaw. Single main menu shows all actions with state. Sub-screens handle individual configuration flows.

## Design Principle: Shared Config Management

All setup and configuration operations are implemented in `internal/config/` for reuse:

| Entry Point | Uses |
|-------------|------|
| `zlaw setup` (interactive TUI) | `internal/config/bootstrap.go` |
| `zlaw init` (non-interactive CLI) | `internal/config/bootstrap.go` |

This ensures consistent behavior across all entry points. See `docs/design/interactive_setup.md` for full design.

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
│  Agent: [assistant ▼]  (3)                 │
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

## Scope

### In Scope
- `cmd/zlaw/setup/` package
- `zlaw setup` command
- Main menu with Bootstrap, Agents, Global sections
- Agent selector dropdown with count
- Sub-screens: bootstrap, create agent, configure LLM, configure adapter, edit identity/soul, manage skills, manage secrets, summary
- Bubble Tea (`github.com/charmbracelet/bubbletea`)

### Out of Scope
- Non-interactive mode
- cli adapter preset
- Model fetch API (defer)
- Multi-agent batch operations

## Phases

| Phase | File | Contents |
|-------|------|----------|
| 01 | `01-dependencies.md` | Add Bubble Tea to go.mod |
| 02 | `02-project-structure.md` | Package layout, shared state types |
| 03 | `03-main-menu.md` | Main menu screen with sections |
| 04 | `04-bootstrap.md` | Bootstrap screen |
| 05 | `05-agent.md` | Agent creation/deletion |
| 06 | `06-llm.md` | LLM configuration + secret setup |
| 07 | `07-adapter.md` | Adapter configuration + secret setup |
| 08 | `08-agent-files.md` | Edit identity/soul, manage skills |
| 09 | `09-secrets.md` | Secrets management |
| 10 | `10-summary.md` | Summary screen |
| 11 | `11-integration.md` | CLI wiring, integration test |

## Current State

### Implemented (All Phases Complete)
- Setup config package: `internal/config/bootstrap.go`, `internal/config/setup.go`
- `zlaw setup` command with all screens (phases 01-11)
- Bubble Tea wizard with main menu, bootstrap, agent, LLM, adapter, identity, soul, skills, secrets, summary screens
- `cmd/zlaw/setup/setup_test.go` integration tests

### Not Implemented
- None — all phases complete!

## Design References

- `docs/design/interactive_setup.md` — menu structure, item states, flows
- `docs/design/llm_presets.md` — LLM preset pattern
- `docs/design/agent_secrets.md` — secrets injection
- `docs/design/channel_adapter.md` — adapter presets
- `docs/design/agent_lifecycle.md` — agent home structure