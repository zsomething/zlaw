# Agent: Channel Adapters

## Overview

Channel adapters connect agents to external communication channels. Each adapter handles a specific protocol (Telegram, CLI, webhook, etc.) and translates between external messages and agent sessions.

## Architecture

```
[External Channel] ──► [Adapter] ──► [Session Manager] ──► [Agent]
                          │                               │
                          ◄─────────────────────────────── [Push]
```

Adapters:
- **Receive** messages from external channels
- **Route** to agent via session manager
- **Push** responses back to the channel

## Built-in Adapters

| Adapter | Protocol | Description |
|---------|----------|-------------|
| `telegram` | Long-polling | Telegram Bot API |
| `cli` | stdin/stdout | Interactive terminal |
| `daemon` | Unix socket | Background process mode |

## Adapter Interface

Adapters implement:

```go
// Pusher sends messages to external channels
type Pusher interface {
    Push(ctx context.Context, address string, message string) error
}

// Runner starts the adapter loop
type Runner interface {
    Run(ctx context.Context) error
}
```

## Configuration

Adapters are configured in `agent.toml`:

```toml
[[adapter]]
backend = "telegram"
client_config = {
    bot_token = "$TELEGRAM_BOT_TOKEN",
}
```

Multiple adapters can be enabled on one agent:


```toml
[[adapter]]
backend = "telegram"
client_config = { bot_token = "$TELEGRAM_BOT_TOKEN" }

[[adapter]]
backend = "slack"
client_config = { bot_token = "$SLACK_BOT_TOKEN" }
```

See [llm_presets.md](./llm_presets.md) for the preset pattern.

## Session Model

Each external chat/session maps to a deterministic session ID. This allows:
- Conversation history per chat
- `/clear` to start fresh while keeping session ID
- Stateless architecture (can restart agent without losing context)

## Slash Commands

Adapters intercept commands starting with `/` before routing to agent:
- `/clear` — archive session, start fresh
- `/history` — show recent conversation
- Custom commands registered per agent

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent startup with adapters
- [command_line.md](./command_line.md) — CLI adapter usage
- [slashcmd package](../internal/slashcmd/) — slash command registry