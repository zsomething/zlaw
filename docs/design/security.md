# Security Model

## Agent Identity

Each agent has an Ed25519 keypair for cryptographic identity. This is core to the security model:

- **Agent keypair** — generated at agent creation time, stored at `$ZLAW_AGENT_HOME/identity.key`
- **Public key** — included in registration message, stored in hub registry
- **Message signing** — agents sign task envelopes, hub verifies signatures
- **NATS authentication** — agents authenticate to NATS using their keypair

### Design Requirements

1. Each agent generates a keypair on first run (self-sovereign identity)
2. Public key is registered with hub at connection time
3. Task envelopes include signature over payload (From + To + Task + SessionID)
4. Receiving agents verify signature against sender's public key from registry
5. Subprocess isolation prevents secret leakage

## NATS ACL

Hub enforces per-agent publish/subscribe at broker layer. No business logic required.

Default permissions (all agents equal):
- **Subscribe**: `zlaw.registry`, `agent.<id>.inbox` (own only), `_INBOX.>`, `$JS.API.>`
- **Publish**: `agent.*.inbox` (delegate to any), `zlaw.registry`, `_INBOX.>`, `$JS.API.>`

## Secret Isolation

Secrets are injected as environment variables by ctl, not file paths. Agent cannot read secret files.

Flow:
1. ctl reads `secrets.toml` (operator-managed)
2. Reads `zlaw.toml` env_vars mapping for agent
3. Injects env vars: `MINIMAX_API_KEY=sk-...`, etc.
4. Agent receives only env vars — no file path exposed

See [agent_credentials.md](./agent_credentials.md) for details.

## Subprocess Isolation (Planned)

When agents spawn subprocesses (e.g., bash tool), secret env vars should be **filtered** — not passed through.

Planned implementation: subprocesses inherit only essential runtime vars (`ZLAW_AGENT_HOME`, `PATH`, etc.), excluding secret keys.

This prevents compromised subprocesses from accessing agent secrets.

## Self-Protection

Hub rejects lifecycle requests (stop/delete) from the target agent itself.

## Audit Log

Append-only. Every tool call, A2A message, user interaction logged with trace ID.

## Prompt Injection Mitigation

Cross-agent messages verified at transport layer before reaching LLM context.

## No Ambient Authority

Agents cannot publish outside ACL, cannot impersonate others.

## See Also

- [agent_credentials.md](./agent_credentials.md) — secret injection design
- [constraints.md](./constraints.md) — hard rules
