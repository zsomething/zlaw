# Built-in tools

The agent loop runs up to 20 tool calls per turn. Multiple tool calls requested in a single model response execute concurrently. Results are size-capped via `max_result_bytes` in `agent.toml`.

---

## Tool reference

| Tool | Description |
|------|-------------|
| `time` | Returns the current date and time |
| `read` | Reads a file; supports `offset` and `limit` for large files |
| `write` | Writes a file, creating parent directories as needed |
| `edit` | Targeted string-replace within a file |
| `glob` | Finds files matching a glob pattern |
| `grep` | Regex search over file contents with line numbers |
| `bash` | Runs a shell command (configurable timeout, max 300 s) |
| `web_fetch` | Fetches a URL and returns the body |
| `web_search` | Searches the web and returns results |
| `http_request` | Makes an arbitrary HTTP request with custom method, headers, and body |
| `memory_save` | Persists a fact with optional tags; upserts by ID |
| `memory_recall` | Searches stored memories by meaning (semantic) or keyword |
| `memory_delete` | Removes a memory by ID |
| `cronjob_list` | Lists all scheduled cron jobs |
| `cronjob_create` | Creates a new cron job |
| `cronjob_delete` | Removes a cron job by ID |
| `configure` | Updates runtime-configurable agent settings (e.g. model override) |

---

## Tool allowlist

By default all tools are available. Restrict access in `agent.toml`:

```toml
[tools]
allowed          = ["read", "bash", "memory_save", "memory_recall"]
max_result_bytes = 65536
```

---

## Resilience

- **Retry with backoff** — failed LLM calls retry automatically; `Retry-After` headers are respected on 429 responses
- **Overload handling** — HTTP 529 (backend at capacity) retries with a longer delay than rate-limit errors
- **Tool error isolation** — a tool returning an error is reported back to the model as a tool result; the agent loop continues
