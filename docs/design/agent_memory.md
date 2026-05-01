# Agent: Long-Term Memory

## Overview

Agents persist facts and information to long-term memory using a Markdown file store. Memories are recalled during context building to provide relevant background.

## Storage Format

Each memory is a Markdown file with YAML frontmatter in `$ZLAW_AGENT_HOME/memories/<id>.md`:

```yaml
---
id: project-alpha
created_at: 2024-01-15T10:00:00Z
updated_at: 2024-01-20T14:30:00Z
tags:
  - project
  - priority
---
Project Alpha is the current focus. Key stakeholders: alice, bob.
```

## Memory Operations

| Operation | Description |
|-----------|-------------|
| Save | Creates or overwrites memory by ID. Accepts content and optional tags. |
| Delete | Removes memory by ID. |
| List | Returns all memories (no guaranteed order). |
| Search | Keyword search over content and tags (case-insensitive). |

## Context Integration

During context building:
1. Agent may query memories relevant to current task
2. Semantic search available if vector store enabled
3. Results injected into system prompt

See [agent_contexts.md](./agent_contexts.md) for context integration details.

## Tools

| Tool | Description |
|------|-------------|
| `memory_save` | Create or update a memory (upsert by ID) |
| `memory_recall` | Search memories by keyword or semantics |
| `memory_delete` | Remove a memory by ID |

## Storage Backend

Default: Markdown file store (`memories/` directory) — keyword search only.

Optional: Vector store for semantic search (chroma-go).

### Semantic Search Requirements

When enabled, requires separate embedder configuration in `agent.toml`:

```toml
[memory.embedder]
backend = "openai"              # preset name
model = "text-embedding-3-small" # embedding model
auth_profile = "openai-embed"   # separate credentials profile
```

The embedder uses its own credentials (distinct from LLM auth profile). Markdown files remain the source of truth; vector index is regenerable.

## See Also

- [agent_contexts.md](./agent_contexts.md) — context building
- [agent_skills.md](./agent_skills.md) — skill system comparison