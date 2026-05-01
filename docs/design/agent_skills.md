# Agent: Markdown Skills

## Overview

Skills are markdown files that extend an agent's capabilities. They provide context, guidelines, and examples that get injected into the system prompt.

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
- `name` — skill identifier
- `description` — when to activate (used by agent to decide)

**Body** — markdown content injected into system prompt when relevant.

## Discovery

Two-level resolution:

| Level | Path | Purpose |
|-------|------|---------|
| Global | `$ZLAW_HOME/skills/<name>/SKILL.md` | Shared across all agents |
| Agent-local | `$ZLAW_HOME/agents/<id>/skills/<name>/SKILL.md` | Agent-specific overrides |

Agent-local wins on name conflict.

## How Skills Are Used

When agent decides to use a skill:
1. Load SKILL.md file
2. Parse frontmatter for name/description
3. Inject body content into system prompt

Skill activation is agent-driven (based on description matching).

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
| Execution | Injected into prompt | Executed by agent |
| Activation | Agent-driven | Tool call |
| Location | Filesystem | Compiled |

## See Also

- [agent_tools.md](./agent_tools.md) — built-in tools
- [agent_contexts.md](./agent_contexts.md) — how skills integrate into context