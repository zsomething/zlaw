# Security Model

## Agent Identity

Each agent has a keypair (NKeys). Hub verifies on connect. Messages signed.

## NATS ACL

Hub enforces per-agent publish/subscribe at broker layer. No business logic required.

Default permissions (all agents equal):
- **Subscribe**: `zlaw.registry`, `agent.<id>.inbox` (own only), `_INBOX.>`, `$JS.API.>`
- **Publish**: `agent.*.inbox` (delegate to any), `zlaw.registry`, `_INBOX.>`, `$JS.API.>`

## Credential Isolation

Agents never read `credentials.toml`. Hub injects only referenced profile as env vars.

Flow:
1. Hub reads `agents/<id>/credentials.toml` at spawn
2. Extracts only the profiles the agent needs
3. Writes to `run/credentials/<id>.toml`
4. Sets `ZLAW_CREDENTIALS_FILE` env var

See [agent_credentials.md](./agent_credentials.md) for details.

## Self-Protection

Hub rejects lifecycle requests (stop/delete) from the target agent itself.

## Audit Log

Append-only. Every tool call, A2A message, user interaction logged with trace ID.

## Prompt Injection Mitigation

Cross-agent messages verified at transport layer before reaching LLM context.

## No Ambient Authority

Agents cannot publish outside ACL, cannot impersonate others.

## See Also

- [agent_credentials.md](./agent_credentials.md) — credential injection
- [constraints.md](./constraints.md) — hard rules
- [plans/separation.md](../plans/separation.md) — current violations