# zlaw

A standalone agentic assistant binary in Go. ReAct loop, any OpenAI-compatible or Anthropic backend, serious context management.

## Status

Phase 1 — standalone `zlaw-agent`. No hub, no NATS, no inter-agent routing yet.

---

## Context engineering

**Long-term memory.** Facts saved with `memory_save` write to plain markdown files under `$ZLAW_HOME/memories/`. Every new session loads them into the system prompt automatically. Enable `proactive_memory_save` and the agent decides what to save without being asked.

**Token budget and pruning.** Set a token limit; as the window fills, old turns get summarised first. If that isn't enough, the agent prunes in layers: extended thinking blocks, then tool outputs, then full conversation turns. Token counts come from the actual API response, not a character estimate.

**Prompt caching (Anthropic).** The system prompt splits into two cached layers. Framework instructions form the stable head with the highest cache hit rate. Personality files get their own checkpoint and reload on file change without invalidating the first layer. Memories load after both, uncached, since they change each session.

**Session prefill.** Inject working directory, current time, or file contents into the first user message of each session. Keeps the system prompt cache clean.

---

## Features

- **ReAct loop** — runs up to 20 iterations per turn; stops when done or when tools return nothing new
- **Any LLM backend** — Anthropic native, or any OpenAI-compatible endpoint (Minimax, OpenRouter, self-hosted)
- **Parallel tool execution** — multiple tool calls in a single turn run concurrently
- **Streaming** — tokens stream to the terminal as they arrive
- **Long-term memory** — `memory_save` writes to plain markdown files; `memory_recall` searches them; all memories load into the system prompt at session start
- **Session history** — conversations persist as JSONL under `$ZLAW_HOME/sessions/<agent>/`; resume any past session by ID
- **Personality hot-reload** — edit `SOUL.md` or `IDENTITY.md`; the change takes effect on the next message
- **Tool guardrails** — allowlist, per-tool result size cap, configurable shell timeout
- **Resilient** — automatic retry with backoff; respects `Retry-After` on 429

### Built-in tools

| Tool | What it does |
|------|-------------|
| `current_time` | Returns the current date and time |
| `read_file` | Reads a file with optional offset/limit |
| `write_file` | Writes a file, creating parent directories |
| `edit_file` | Targeted string-replace within a file |
| `glob` | Finds files by glob pattern |
| `grep` | Regex search over file contents with line numbers |
| `bash` | Runs a shell command (configurable timeout, max 300 s) |
| `web_fetch` | Fetches a URL and returns the body |
| `web_search` | Searches the web and returns results |
| `http_request` | Makes an arbitrary HTTP request |
| `memory_save` | Persists a fact with optional tags (upserts by ID) |
| `memory_recall` | Keyword/tag search over stored memories |
| `memory_delete` | Removes a memory by ID |

---

## Quick start

```sh
export ZLAW_HOME=~/.zlaw

zlaw-agent auth login

# Run with a named agent (reads $ZLAW_HOME/agents/<name>/agent.toml)
zlaw-agent run --agent default
```

---

## Configuration

Each agent lives in a directory with three files:

| File | Purpose |
|------|---------|
| `agent.toml` | LLM backend, model, auth profile, context settings, tool allowlist |
| `SOUL.md` | Personality and behavioural guidelines (optional) |
| `IDENTITY.md` | Agent identity and role description (optional) |

### Minimal `agent.toml`

```toml
[agent]
name = "myagent"

[llm]
backend      = "anthropic"
model        = "claude-sonnet-4-5"
auth_profile = "anthropic-default"
max_tokens   = 4096
timeout_sec  = 60
```

### Context management

```toml
[llm]
# Hard token budget for the message history.
# Oldest turns are pruned when the estimate exceeds this value.
context_token_budget = 80000

# Fraction of budget at which summarisation triggers before pruning.
# 0 = summarisation disabled.
context_summarize_threshold = 0.8

# How many of the oldest turns to collapse per summarisation pass.
context_summarize_turns = 10

# Route summarisation to a cheaper/faster model (same backend and auth profile).
context_summarize_model = "claude-haiku-4-5-20251001"

# Pruning strategies applied in order after summarisation.
# strip_thinking → strip_tool_results → drop_pairs
context_prune_levels = ["strip_thinking", "strip_tool_results", "drop_pairs"]

# Anthropic prompt caching (default: true on Anthropic backend).
# prompt_caching = true

# Cap the [Memories] block injected into the system prompt.
max_memory_tokens = 2000
```

### Context prefill

Inject dynamic context into the first user message of each new session:

```toml
[context]
prefill = ["cwd", "datetime", "file:NOTES.md"]
```

Supported sources: `cwd` (working directory), `datetime` (RFC3339 timestamp), `file:<path>` (relative to agent directory).

### Long-term memory

Memory files are stored as `$ZLAW_HOME/memories/<agent>/<id>.md` with YAML frontmatter — human-readable and version-controllable. Enable proactive saving with a sticky instruction block:

```toml
[sticky]
proactive_memory_save = true
```

When enabled, a `[Memory behavior]` instruction is prepended to every system prompt as cache checkpoint 1, telling the agent to call `memory_save` whenever it learns something worth retaining (user preferences, project facts, recurring context).

### Tool allowlist

```toml
[tools]
allowed = ["read_file", "bash", "memory_save", "memory_recall", "memory_delete"]
max_result_bytes = 65536
```

---

## Runtime paths

All runtime paths derive from `$ZLAW_HOME` (defaults to `$PWD`):

| Path | Contents |
|------|---------|
| `$ZLAW_HOME/agents/<name>/` | Agent config files |
| `$ZLAW_HOME/sessions/<name>/` | Durable session history (JSONL) |
| `$ZLAW_HOME/memories/<name>/` | Long-term memory files (Markdown) |
| `$ZLAW_HOME/credentials.toml` | Auth profiles (mode 0600) |

Override the credentials path with `ZLAW_CREDENTIALS_FILE`.
