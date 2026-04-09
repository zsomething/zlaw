# zlaw

A standalone agentic assistant binary written in Go. Runs a ReAct loop (reason → tool call → observe → repeat) driven by any OpenAI-compatible LLM backend.

## Status

Phase 1 — standalone `zlaw-agent`. No hub, no NATS, no inter-agent routing yet.

## Features

- **ReAct agentic loop** — LLM reasons, calls tools, observes results, repeats until done
- **OpenAI-compatible backends** — Minimax, OpenRouter, or any OpenAI-compat endpoint
- **Auth profiles** — API key or OAuth2 client-credentials; stored in `$ZLAW_HOME/credentials.toml`, never in agent config
- **Durable session history** — conversations persist as JSONL under `$ZLAW_HOME/sessions/<agent>/`
- **Context management** — token-budget pruning and optional LLM-based summarisation of old turns
- **LLM retry** — exponential backoff with jitter; honours `Retry-After` on 429
- **Streaming responses** — streams LLM output to the terminal as it arrives
- **Tool allowlist** — `tools.allowed` in `agent.toml` restricts which tools an agent can use
- **Hot-reload** — `agent.toml`, `SOUL.md`, and `IDENTITY.md` are re-read on file change without restart
- **CLI adapter** — interactive REPL or stdin pipe mode; auto-detected

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
| `agent.toml` | LLM backend, model, auth profile, tool allowlist |
| `SOUL.md` | Personality and behavioural guidelines (optional) |
| `IDENTITY.md` | Agent identity and role description (optional) |

Minimal `agent.toml`:

```toml
[agent]
name = "myagent"

[llm]
backend = "minimax"
model   = "minimax-m2.7"
auth_profile = "minimax-default"
max_tokens   = 4096
timeout_sec  = 60

[tools]
# Leave empty to allow all tools, or list names to restrict:
# allowed = ["read_file", "bash"]
```

## Runtime paths

All runtime paths derive from `$ZLAW_HOME` (defaults to `$PWD`):

| Path | Contents |
|------|---------|
| `$ZLAW_HOME/agents/<name>/` | Agent config files |
| `$ZLAW_HOME/sessions/<name>/` | Durable session history (JSONL) |
| `$ZLAW_HOME/credentials.toml` | Auth profiles (mode 0600) |

Override the credentials path with `ZLAW_CREDENTIALS_FILE`.
