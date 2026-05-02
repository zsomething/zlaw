# Implementation Plan: Interactive Setup Wizard

## Overview

Implement `zlaw setup` — a menu-based TUI for configuring zlaw. Single main menu shows all actions with state. Sub-screens handle individual configuration flows.

## Goals

1. **Menu-based navigation** — single menu, sub-screens replace menu
2. **State visibility** — every item shows current state (configured, missing, etc.)
3. **Agent isolation** — agent items only visible when agent selected
4. **Bubble Tea TUI** — interactive with keyboard navigation

## Scope

### In Scope
- New `cmd/zlaw/setup/` package with Bubble Tea wizard
- `zlaw setup` command
- Main menu with Bootstrap, Agents, Global sections
- Agent selector dropdown
- Sub-screens: bootstrap, create agent, configure LLM, configure adapter, edit files, manage skills, manage secrets, summary
- Bubble Tea dependency (`github.com/charmbracelet/bubbletea`)

### Out of Scope
- Non-interactive mode
- cli adapter preset
- Model fetch API (manual entry fallback)
- Multi-agent batch operations

## Files

| File | Contents |
|------|----------|
| `00-overview.md` | This file |
| `01-dependencies.md` | Phase 1: Add Bubble Tea dependency |
| `02-project-structure.md` | Phase 2: Project structure, state types |
| `03-main-menu.md` | Phase 3: Main menu screen |
| `04-bootstrap.md` | Phase 4: Bootstrap screen |
| `05-agent.md` | Phase 5: Agent creation |
| `06-llm.md` | Phase 6: LLM configuration |
| `07-adapter.md` | Phase 7: Adapter configuration |
| `08-agent-files.md` | Phase 8: Edit identity/soul, manage skills |
| `09-secrets.md` | Phase 9: Secrets management |
| `10-summary.md` | Phase 10: Summary screen |
| `11-integration.md` | Phase 11: CLI wiring, integration test |

## Current State

### Implemented
- Agent creation via `zlaw init --agent <id>`
- Secret management via `zlaw auth add/list/remove`
- LLM presets in `internal/llm/presets.go`
- Adapter presets in `internal/adapter/preset.go`
- Skills directory in `cmd/zlaw/init.go`

### Not Implemented
- `zlaw setup` command
- Bubble Tea wizard
- Menu navigation model
- Sub-screens

## Design Reference

- `docs/design/interactive_setup.md`