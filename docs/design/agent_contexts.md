# Agent: Context Engineering

## Overview

Each LLM call requires a carefully constructed context. The agent builds this from multiple sources to give the LLM the information it needs to respond effectively.

## Context Components

### 1. System Prompt

Built from multiple layers:

```
System Prompt =
    SOUL.md                    # personality, tone, guidelines
  + IDENTITY.md               # role, capabilities, constraints
  + Sticky blocks             # framework-level injections
  + Tool definitions          # available tools + schemas
  + Memory recall             # relevant memories (if semantic search enabled)
```

### 2. Sticky Blocks

Framework-level instructions injected at the head of every system prompt. Content lives in Go source (not markdown files) so user personality files cannot override them.

Current sticky blocks:
- **self-identity** — "You are agent {agent_id} with roles {agent_roles}"
- **allowed-tools** — list of permitted tools based on config

### 3. Conversation History

Token-limited window of recent turns. Pruned when it exceeds `context_token_budget`.

```
History = last N turns (token-limited)
```

Pruning strategy:
1. When history exceeds token budget, remove oldest turns
2. If still over budget, apply summarization to remaining turns
3. If summarization also exceeds, apply aggressive pruning (strip tool results, thinking blocks)

### 4. Prefill

Context injected at session start based on `context.prefill` config:

| Source | Content |
|--------|---------|
| `cwd` | Current working directory |
| `datetime` | Current date/time (RFC3339) |
| `file:<path>` | Contents of file relative to agent home |

### 5. Memory Recall

When semantic memory is enabled:
1. Query vector store with session context
2. Inject relevant memory files into context
3. Configurable via `memory.recall_threshold` and `memory.max_memory_tokens`

## Token Budget

The agent tracks token counts to stay within LLM context limits:

```
context_token_budget = max tokens for history + tools + memories

Pruning triggers:
- History exceeds budget → prune oldest turns
- After pruning, still over → summarize middle turns
- After summarization, still over → strip_tool_results → strip_thinking
```

## Context Optimization Pipeline

```
Input: fresh history
  │
  ├─► Within budget? ──YES──► Use as-is
  │
  ├─► NO: Prune oldest turns
  │
  ├─► Still over? ──NO──► Done
  │
  ├─► YES: Summarize middle
  │
  ├─► Still over? ──NO──► Done
  │
  ├─► YES: Aggressive prune (strip results, thinking)
  │
  └─► Done
```

## Tool Definitions

Injected into system prompt as JSON schema. Only tools in `tools.allowed` are included (if allowlist is configured).

## See Also

- [agent_standalone.md](./agent_standalone.md) — standalone agent overview
- [agent_tools.md](./agent_tools.md) — tool reference