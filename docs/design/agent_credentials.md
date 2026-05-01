# Agent: Credentials

## Overview

Credentials are injected into agents at spawn time via environment variables. The agent does not read raw secrets — it only receives a path to a credential profile.

## Design Goals

1. **Secret isolation** — agents never read raw secrets; hub handles credential files
2. **Minimal exposure** — agent only gets the profiles it needs, not all profiles
3. **No credential exfiltration** — agent cannot read or expose credential file contents

## Injection Flow

```
Hub                                Agent
 │                                    │
 │ Read agents/<id>/agent.toml        │
 │ Determine required auth profiles   │
 │                                    │
 │ Read agents/<id>/credentials.toml  │
 │ Extract needed profiles            │
 │                                    │
 │ Write run/credentials/<id>.toml    │  (hub-owned, agent cannot access before spawn)
 │                                    │
 │ Spawn agent with env:              │
 │   ZLAW_CREDENTIALS_FILE=<path>     │──► Agent reads ONLY this file at startup
 │                                    │
```

## Environment Variable

| Variable | Set by | Purpose |
|----------|--------|---------|
| `ZLAW_CREDENTIALS_FILE` | Hub at spawn | Path to agent's credential profile |

## Agent Usage

```go
// Agent reads credential file via LLM factory
src, err := credentials.NewTokenSourceFromStore(
    os.Getenv("ZLAW_CREDENTIALS_FILE"),
    cfg.AuthProfile,  // e.g., "minimax-default"
)
```

The agent:
- Reads only the profile named in `agent.toml` (`auth_profile` field)
- Cannot enumerate other profiles in the file
- Cannot access `credentials.toml` directly

## Credential File Structure

`run/credentials/<agentID>.toml`:
```toml
[<profile-name>]
api_key = "sk-..."
# or for OAuth2:
client_id = "..."
client_secret = "..."
```

## Auth Profile Resolution

In `agent.toml`:
```toml
[llm]
backend = "minimax"
auth_profile = "minimax-default"
```

The agent looks up `minimax-default` from the injected credentials file.

## Agent Never Sees

- Raw `credentials.toml` in agent directory
- Profiles it doesn't need
- Global credentials

## Current Implementation

The current implementation reads from `agents/<id>/credentials.toml` directly — this is a known violation. See [plans/separation.md](../plans/separation.md) for planned fixes.

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent startup sequence
- [plans/separation.md](../plans/separation.md) — architectural violations