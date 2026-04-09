# zlaw

A standalone agentic assistant binary written in Go. Runs a ReAct loop (reason → tool call → observe → repeat) driven by any OpenAI-compatible or Anthropic LLM backend.

## Status

Phase 1 — standalone `zlaw-agent`. No hub, no NATS, no inter-agent routing yet.

## Features

- **ReAct agentic loop** — LLM reasons, calls tools, observes results, repeats until done
- **Multiple LLM backends** — Anthropic native API, OpenAI-compatible endpoints (Minimax, OpenRouter, etc.)
- **Auth profiles** — API key or OAuth2 client-credentials; stored in `$ZLAW_HOME/credentials.toml`, never in agent config
- **Durable session history** — conversations persist as JSONL under `$ZLAW_HOME/sessions/<agent>/`
- **Context management** — token-budget pruning with cascading levels (`strip_thinking`, `strip_tool_results`, `drop_pairs`) and optional LLM-based summarisation of old turns
- **Prompt caching** — Anthropic backend sends cache checkpoints for sticky blocks and personality sections to reduce cost and latency
- **Sticky context** — framework-level instruction blocks prepended to every system prompt; never overridden by personality files
- **LLM retry** — exponential backoff with jitter; honours `Retry-After` on 429
- **Streaming responses** — streams LLM output to the terminal as it arrives
- **Concurrent tool execution** — tool calls within a single LLM turn run in parallel
- **Tool allowlist** — `tools.allowed` in `agent.toml` restricts which tools an agent can use
- **Tool result truncation** — `tools.max_result_bytes` caps large tool outputs before they reach the context window
- **Context prefill** — configurable session-start preamble (cwd, datetime, file contents) injected into the first user message
- **Hot-reload** — `agent.toml`, `SOUL.md`, and `IDENTITY.md` are re-read on file change without restart
- **CLI adapter** — interactive REPL or stdin pipe mode; auto-detected; `/clear` and `/history` commands

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

## Quick start

```sh
# Set the runtime root (defaults to $PWD)
export ZLAW_HOME=~/.zlaw

# Add credentials
zlaw-agent auth login

# Run with a named agent (reads $ZLAW_HOME/agents/<name>/agent.toml)
zlaw-agent run --agent default

# Or point directly at an agent directory
zlaw-agent run --agent-dir ./agents/default
```

## Agent configuration

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

[tools]
# Leave empty to allow all tools, or list names to restrict:
# allowed = ["read_file", "bash"]
```

### Context management

```toml
[llm]
# Hard token budget for the message history sent to the LLM.
# Oldest turns are pruned when the estimate exceeds this value.
context_token_budget = 80000

# Fraction of budget at which summarisation is triggered before pruning.
# 0 disables summarisation.
context_summarize_threshold = 0.8

# How many of the oldest turns to collapse per summarisation pass (default 10).
context_summarize_turns = 10

# Optional cheaper/faster model for summarisation (same backend and auth profile).
context_summarize_model = "claude-haiku-4-5-20251001"

# Ordered pruning strategies applied after summarisation.
# Supported: "strip_thinking", "strip_tool_results", "drop_pairs"
context_prune_levels = ["strip_thinking", "strip_tool_results", "drop_pairs"]

# Enable Anthropic prompt caching for the system prompt (default true on Anthropic backend).
# prompt_caching = true
```

### Context prefill

Inject dynamic context into the first user message of each new session:

```toml
[context]
prefill = ["cwd", "datetime", "file:NOTES.md"]
```

Supported sources: `cwd` (working directory), `datetime` (RFC3339 timestamp), `file:<path>` (file relative to agent directory).

### Sticky context

Sticky blocks are prepended to every system prompt as the stable head. They live in Go source so personality files cannot override them. Enable built-in blocks via `agent.toml`:

```toml
[sticky]
# proactive_memory_save = true  # coming soon
```

## Runtime paths

All runtime paths derive from `$ZLAW_HOME` (defaults to `$PWD`):

| Path | Contents |
|------|---------|
| `$ZLAW_HOME/agents/<name>/` | Agent config files |
| `$ZLAW_HOME/sessions/<name>/` | Durable session history (JSONL) |
| `$ZLAW_HOME/credentials.toml` | Auth profiles (mode 0600) |

Override the credentials path with `ZLAW_CREDENTIALS_FILE`.
