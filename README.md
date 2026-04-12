# zlaw

A personal AI assistant that runs locally, connects to any LLM, and gets smarter the more you use it.

---

## What it does

**Remembers things.** Memories are saved as plain Markdown files you can read, edit, and version-control. When you enable proactive saving, the agent decides on its own what's worth keeping — user preferences, project context, recurring facts — without being asked. Recall is semantic: queries find relevant memories by meaning, not just matching keywords.

**Runs on a schedule.** Define cron jobs in plain text and the agent executes them automatically — send a morning briefing, check for updates, run a recurring task. Jobs are hot-reloadable without a restart.

**Talks to you on Telegram.** Connect a Telegram bot and the assistant lives in your pocket. Conversations are session-aware; you can have separate threads for different contexts.

**Stays sharp in long conversations.** As the context window fills, the agent summarises old turns to preserve meaning, then prunes selectively — stripping extended thinking first, then tool outputs, then full turns. You set the budget; it manages the window.

**Adapts on the fly.** Edit the agent's personality or behaviour and the change takes effect on the next message — no restart needed. The same applies to scheduled jobs, tool config, and runtime model overrides.

---

## Features

- **Any LLM backend** — Anthropic native, or any OpenAI-compatible endpoint (Minimax, OpenRouter, Ollama, self-hosted)
- **Long-term memory** — Markdown files on disk, human-readable and git-trackable; semantic search via vector index
- **Proactive memory saving** — agent saves what matters without being asked, guided by a sticky instruction block
- **Cron-scheduled tasks** — define recurring agent jobs in `cron.toml`; reloaded without restart
- **Telegram adapter** — full session support over Telegram bot API
- **Hot-reload** — personality files, cron jobs, and runtime model config update live
- **Context window management** — token budget, multi-tier summarisation, layered pruning (thinking → tool results → turns)
- **Prompt caching** — Anthropic backends split the system prompt into stable cached layers for lower latency and cost
- **Session persistence** — conversations stored as JSONL; resume any session by ID
- **Streaming** — tokens stream to the terminal as they arrive
- **Built-in tools** — file read/write/edit, glob, grep, bash, web fetch/search, HTTP requests, memory ops, cron management

---

## Quick start

```sh
# Bootstrap a new agent
zlaw-agent init --name myagent

# Run interactively
zlaw-agent --agent myagent run

# Run as a daemon (Unix socket + optional Telegram)
zlaw-agent --agent myagent serve
```

Credentials live in `~/.zlaw/credentials.toml`. Add a profile:

```sh
zlaw-agent auth login --profile myprofile --type apikey --key <key>
```

---

## Configuration

Each agent is a directory with three files:

| File | Purpose |
|------|---------|
| `agent.toml` | LLM backend, model, memory, tools, adapter, context settings |
| `SOUL.md` | Personality and behavioural guidelines |
| `IDENTITY.md` | Agent role and identity |

### Minimal `agent.toml`

```toml
[agent]
name = "myagent"

[llm]
backend      = "openrouter"
model        = "openai/gpt-4o"
auth_profile = "openrouter"
max_tokens   = 4096
```

### Memory

```toml
[sticky]
proactive_memory_save = true   # agent saves facts without being asked

[memory.embedder]
backend      = "openrouter"
model        = "openai/text-embedding-3-small"
auth_profile = "openrouter"    # omit to reuse the LLM auth profile
```

Memory files are stored under `$ZLAW_HOME/memories/<agent>/` as plain Markdown — readable, editable, and version-controllable. The vector index is a local cache derived from those files; delete it to force a rebuild.

### Context management

```toml
[llm]
context_token_budget         = 80000   # hard limit on history size
context_summarize_threshold  = 0.8     # summarise at 80% of budget
context_summarize_turns      = 10      # turns to collapse per pass
context_summarize_model      = "openai/gpt-4o-mini"   # cheaper model for summarisation
context_prune_levels         = ["strip_thinking", "strip_tool_results", "drop_pairs"]
max_memory_tokens            = 2000    # cap the [Memories] block in the system prompt
```

### Scheduled tasks

```toml
# cron.toml (in the agent directory)
[[jobs]]
id       = "morning-briefing"
schedule = "0 8 * * *"
prompt   = "Give me a brief summary of what I should focus on today."
```

---

## Coming soon

Multi-agent routing, inter-agent messaging, and a central hub for orchestrating fleets of specialised agents.
