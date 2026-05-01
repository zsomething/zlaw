# Agent: Markdown Skills

## Overview

Skills are markdown files that extend agent capabilities. They provide context, guidelines, and examples on-demand.

## Skill File Format

```
---
name: <skill-name>
description: <when should this skill be activated?>
---

# Skill Title

## Guidelines
...

## Examples
...
```

**Frontmatter** (YAML):
- `name` — skill identifier, used for matching
- `description` — trigger condition; agent uses this to decide when to activate

**Body** — full content loaded into context only when skill is activated.

## Discovery

Two-level resolution:

| Level | Path | Purpose |
|-------|------|---------|
| Global | `$ZLAW_HOME/skills/<name>/SKILL.md` | Shared across all agents |
| Agent-local | `$ZLAW_HOME/agents/<id>/skills/<name>/SKILL.md` | Agent-specific overrides |

Agent-local wins on name conflict.

## How Skills Are Used

Skills are **not** pre-loaded into context. Only name + description are registered.

When agent decides to activate a skill:
1. Agent matches task against registered skill descriptions
2. If matched, loads full SKILL.md body content
3. Injects body into system prompt or relevant context

```
Skill Discovery (at startup):
  name + description → registered in agent

Skill Activation (per task):
  task → match description → load body → inject into context
```

## Example: `debug-go`

```
---
name: debug-go
description: Use when user asks about Go debugging, panics, or runtime errors
---

# Go Debugging Guide

## Reading Stack Traces
...

## Common Patterns
- nil pointer dereference
- index out of bounds
...
```

## Skills vs Built-in Tools

| Aspect | Skills | Built-in Tools |
|--------|--------|----------------|
| Format | Markdown | Go code |
| Context loading | On-demand (triggered by description) | Always available |
| Content | Guidelines/examples injected into prompt | Tool schema injected |
| Activation | Agent-driven (description matching) | Tool call |

## See Also

- [agent_tools.md](./agent_tools.md) — built-in tools
- [agent_contexts.md](./agent_contexts.md) — how skills integrate into context