# Configuration reference

Each agent is a directory under `$ZLAW_HOME/agents/<name>/` containing three files:

| File | Purpose |
|------|---------|
| `agent.toml` | LLM backend, model, auth profile, context settings, tool allowlist |
| `SOUL.md` | Personality and behavioural guidelines (optional) |
| `IDENTITY.md` | Agent identity and role description (optional) |

Bootstrap a new agent directory with:

```sh
zlaw-agent init --name <name>
```

---

## agent.toml

### `[agent]`

```toml
[agent]
name        = "myagent"
description = ""
```

### `[llm]`

```toml
[llm]
backend      = "openrouter"           # named preset or custom (see Backends)
model        = "openai/gpt-4o"
auth_profile = "openrouter"           # profile name in credentials.toml
max_tokens   = 4096
timeout_sec  = 60

# Hard limit on message history size (in tokens).
# Oldest turns are pruned when the estimate exceeds this value.
context_token_budget = 80000

# Fraction of budget at which summarisation triggers before pruning.
# 0 = summarisation disabled.
context_summarize_threshold = 0.8

# Number of oldest turns to collapse per summarisation pass.
context_summarize_turns = 10

# Route summarisation to a cheaper/faster model (same backend and auth profile).
context_summarize_model = "openai/gpt-4o-mini"

# Pruning strategies applied in order after summarisation.
# strip_thinking → strip_tool_results → drop_pairs
context_prune_levels = ["strip_thinking", "strip_tool_results", "drop_pairs"]

# Anthropic prompt caching (default: true on Anthropic backend).
# prompt_caching = true

# Cap the [Memories] block injected into the system prompt.
max_memory_tokens = 2000
```

### `[memory]`

```toml
[sticky]
proactive_memory_save = true   # agent saves facts without being asked

[memory.embedder]
backend      = "openrouter"
model        = "openai/text-embedding-3-small"
auth_profile = "openrouter"    # omit to reuse the LLM auth profile
```

Memory files are stored as `$ZLAW_HOME/memories/<agent>/<id>.md` with YAML frontmatter. The vector index lives in `.index/` alongside the memory files and is a regenerable cache — delete it to force a full rebuild.

When `proactive_memory_save` is enabled, a `[Memory behavior]` instruction is prepended to every system prompt, telling the agent to call `memory_save` whenever it learns something worth retaining (user preferences, project facts, recurring context).

### `[context]`

Inject dynamic context into the first user message of each new session:

```toml
[context]
prefill = ["cwd", "datetime", "file:NOTES.md"]
```

Supported sources:

| Source | Injects |
|--------|---------|
| `cwd` | Current working directory |
| `datetime` | Current date and time (RFC3339) |
| `file:<path>` | Contents of a file relative to the agent directory |

Prefill keeps the system prompt cache clean by putting volatile context (time, directory) in the user turn instead.

### `[tools]`

```toml
[tools]
allowed          = ["read_file", "bash", "memory_save", "memory_recall", "memory_delete"]
max_result_bytes = 65536
```

`allowed` is an allowlist of tool names. Omit or leave empty to allow all built-in tools. `max_result_bytes` caps the size of any single tool result returned to the model.

### `[adapter]`

```toml
[adapter]
type = "cli"   # "cli" or "telegram"
```

### `[serve]`

```toml
[serve]
shutdown_timeout = 60   # seconds to wait for in-flight turns before force-cancel
```

---

## Backends

Named presets resolve to a base URL and API format. Override either in `agent.toml` if needed.

| Preset | Format | Base URL |
|--------|--------|----------|
| `anthropic` | Anthropic | `https://api.anthropic.com` |
| `minimax` | Anthropic-compat | `https://api.minimax.io/anthropic` |
| `minimax-openai` | OpenAI-compat | `https://api.minimax.io/v1` |
| `minimax-cn` | Anthropic-compat | `https://api.minimaxi.com/anthropic` |
| `minimax-cn-openai` | OpenAI-compat | `https://api.minimaxi.com/v1` |
| `openrouter` | OpenAI-compat | `https://openrouter.ai/api/v1` |

Custom endpoint:

```toml
[llm]
backend  = "minimax"
base_url = "https://my-proxy.example.com/anthropic"   # overrides preset URL
```

---

## Credentials

Credentials live in `$ZLAW_HOME/credentials.toml` (mode 0600). Override the path with `ZLAW_CREDENTIALS_FILE`.

```toml
[profiles.myprofile]
type = "apikey"
key  = "${MY_API_KEY}"   # env-var expansion supported
```

Add a profile interactively:

```sh
zlaw-agent auth login --profile myprofile --type apikey --key <key>
```

OAuth2 (`client_credentials` grant) is also supported:

```toml
[profiles.myprofile]
type          = "oauth2"
token_url     = "https://auth.example.com/token"
client_id     = "${CLIENT_ID}"
client_secret = "${CLIENT_SECRET}"
```

---

## Runtime paths

All paths derive from `$ZLAW_HOME` (defaults to `$PWD`):

| Path | Contents |
|------|---------|
| `$ZLAW_HOME/agents/<name>/` | Agent config files |
| `$ZLAW_HOME/sessions/<name>/` | Durable session history (JSONL) |
| `$ZLAW_HOME/memories/<name>/` | Long-term memory files (Markdown) |
| `$ZLAW_HOME/memories/<name>/.index/` | Vector index cache (regenerable) |
| `$ZLAW_HOME/credentials.toml` | Auth profiles |

---

## Context engineering notes

**Token budget and pruning.** Token counts come from the actual API response, not a character estimate. When the budget is exceeded, the pipeline runs: summarise oldest turns → strip extended thinking blocks → strip tool result bodies → drop full turn pairs. Each level only runs if the previous wasn't enough.

**Prompt caching (Anthropic).** The system prompt splits into stable cached layers: framework instructions first (highest cache hit rate), then personality files. Memories load last, uncached, since they change each session. Editing `SOUL.md` or `IDENTITY.md` invalidates only the personality layer, not the framework layer.

**Session prefill.** By putting volatile context (current time, working directory) in the first user message rather than the system prompt, the system prompt cache is preserved across sessions.
