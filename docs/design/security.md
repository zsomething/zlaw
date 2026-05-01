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
5. Subprocess isolation prevents credential leakage

### Future: Hub ACL with Keypairs

Hub may use agent public keys for per-agent NATS ACL (instead of token-based). This enables:
- Cryptographic verification of agent identity at NATS layer
- Revocation by removing key from hub's trust store
- No shared secrets (tokens) to manage

## NATS ACL

Hub enforces per-agent publish/subscribe at broker layer. No business logic required.

Default permissions (all agents equal):
- **Subscribe**: `zlaw.registry`, `agent.<id>.inbox` (own only), `_INBOX.>`, `$JS.API.>`
- **Publish**: `agent.*.inbox` (delegate to any), `zlaw.registry`, `_INBOX.>`, `$JS.API.>`

## Credential Isolation

Credentials are injected as environment variables, not file paths. Agent cannot read credential files.

Flow:
1. Hub reads `agents/<id>/credentials.toml` at spawn
2. Extracts only the profiles the agent needs
3. Injects as env vars: `MINIMAX_API_KEY=sk-...`, etc.
4. Agent reads env vars directly — no file path exposed

See [agent_credentials.md](./agent_credentials.md) for details.

## Subprocess Isolation (Planned)

When agents spawn subprocesses (e.g., bash tool), credential env vars should be **filtered** — not passed through.

Planned implementation: subprocesses inherit only essential runtime vars (`ZLAW_AGENT_HOME`, `PATH`, etc.), excluding credential keys.

This prevents compromised subprocesses from accessing agent credentials.

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