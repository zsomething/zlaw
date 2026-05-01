# Agent: P2P Delegation

## Overview

Agents delegate tasks to other agents via NATS. Hub provides routing + ACL only; business logic lives in agents.

## Delegation Flow

```
Agent A                      NATS                          Agent B
   │                                                      │
   │ Publish TaskEnvelope                                │
   │ ─────────────────────────────────────────────────► │
   │                                                      │
   │                                            Agent B processes task
   │                                                      │
   │◄─────────────────────────────────────────────────── │
   │ Reply (to reply_to inbox)                            │
```

## TaskEnvelope

JSON payload published to `agent.<target_id>.inbox`:

```json
{
  "from": "alice",
  "to": "bob",
  "task": "fetch_weather",
  "context": {
    "location": "San Francisco"
  },
  "reply_to": "agent.alice.inbox",
  "session_id": "abc123",
  "trace_id": "xyz789"
}
```

Fields:
- `from` — delegating agent ID
- `to` — target agent ID
- `task` — task identifier (agent-defined)
- `context` — task-specific data
- `reply_to` — inbox for reply (agent.<from_id>.inbox)
- `session_id` — session context (for trace linking)
- `trace_id` — distributed trace ID (for observability)

## agent_delegate Tool

Built-in tool that publishes the TaskEnvelope:

```
agent_delegate(id: string, task: string, context: object)
```

Validation:
- Target agent must be registered (via Registry check)
- Caller must have publish permission to `agent.<id>.inbox` (ACL enforced by NATS)

In standalone mode (no hub): returns error immediately.

## P2P Permissions

All agents have equal permissions:
- **Subscribe**: `zlaw.registry`, `agent.<id>.inbox` (own only), `_INBOX.>`, `$JS.API.>`
- **Publish**: `agent.*.inbox` (delegate to any), `zlaw.registry`, `_INBOX.>`, `$JS.API.>`

See [hub.md](./hub.md) for ACL details.

## Reply Pattern

Agents can reply to delegation requests by publishing to the `reply_to` inbox specified in the envelope.

Reply format (agent-defined, typically JSON):
```json
{
  "result": "...",
  "error": null
}
```

## Session Context

When delegating:
- `session_id` links to the originating session for audit
- `trace_id` enables distributed tracing across agent hops
- Sub-agent typically starts fresh session (future: propagate session context)

## See Also

- [hub.md](./hub.md) — NATS ACL and hub internals
- [agent_standalone.md](./agent_standalone.md) — agent overview