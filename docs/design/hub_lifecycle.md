# Hub Lifecycle

## Overview

Hub is started by ctl (via executor). Hub's responsibilities during lifecycle events are documented here.

## Startup

When ctl starts hub:

1. ctl loads zlaw.toml
2. ctl starts embedded NATS server
3. ctl starts hub process
4. Hub subscribes to registry subject
5. Hub loads agent entries from zlaw.toml (for validation)
6. Hub validates agent directories are absolute paths

## Agent Registration

When an agent connects to hub:

1. Agent publishes registration to `zlaw.registry`
2. Hub receives registration via registry subscription
3. Hub stores agent info in registry:
   - ID, version, capabilities, roles
   - Public key (for message signing)
   - Connection status (connected/disconnected)
4. Hub uses registry for message routing

## Secret Injection

Handled by ctl via executor at spawn. See [agent_secrets.md](./agent_secrets.md) for details.

## Agent Disconnection

When hub detects agent disconnection (missed heartbeats):

1. Hub marks agent as `disconnected` in registry
2. Hub may retain agent in registry for message routing (agents may reconnect)

## Shutdown

When ctl stops hub:

1. ctl sends stop signal to hub process
2. Hub deregisters all agents from registry
3. Hub closes NATS connections
4. ctl stops NATS server

## See Also

- [ctl_supervisor.md](./ctl_supervisor.md) — ctl and executor design
- [agent_lifecycle.md](./agent_lifecycle.md) — agent lifecycle
- [hub.md](./hub.md) — hub's role in message routing
- [agent_secrets.md](./agent_secrets.md) — secrets and env var injection
- [llm_presets.md](./llm_presets.md) — LLM backend presets
