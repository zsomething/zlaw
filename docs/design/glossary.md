# Glossary

## Agent

Autonomous process that runs the agentic loop (ReAct). Receives input, executes tools, produces output. Owns its filesystem under `$ZLAW_AGENT_HOME`.

**Not**: Hub, ctl, plugin.

## Agentic Loop (ReAct)

Pattern: Input → Build context → LLM call → (tool call → execute → append result → loop) → emit response.

## Hub

Communication broker. Routes messages between agents and provides external interfaces (webhook). Enforces NATS ACL. Does NOT manage agent lifecycle or secrets.

**Not**: Agent orchestrator, process manager.

## ctl

Human operator CLI. Scaffolds agent directories, manages agent lifecycle (create/start/stop/restart/delete), manages secrets, talks to hub via control socket. Uses executor abstraction for execution.

**Not**: Agent, plugin.

## ZLAW_HOME

Root directory convention for local setups. ctl-owned. Contains `zlaw.toml`, `secrets.toml`, `run/`, `nats/`, `agents/`.

**Not**: Agent's home directory.

## ZLAW_AGENT_HOME

Agent's self-contained root. Set via env var. Agent reads this for all its files (sessions, memories, etc.).

**Not**: ZLAW_HOME.

## Secrets (formerly Credentials)

API keys, tokens, and secrets stored in `secrets.toml` (formerly `credentials.toml`). Injected into agents as env vars at spawn time via ctl. Never exposed as file paths to agents.

## Channel Adapter

Component that connects agents to external communication channels (Telegram, CLI, webhook). Translates between external messages and agent sessions.

**Also known as**: Adapter (in code).

## Preset

Static, well-known configuration template stored as Go code. Defines defaults for LLM backends (base_url, api_format, default model) and channel adapters (parse_mode, features). Copied to agent config at creation. Agent.toml can override preset defaults.

**Also known as**: LLMPreset, AdapterPreset.

## Delegation (P2P)

Agent-to-agent communication via NATS. One agent publishes a task to another's inbox. Hub routes but does not orchestrate.

## Session

Conversation context keyed by session ID. Persisted to JSONL files. One agent can have multiple concurrent sessions.

## Skill

Markdown file that extends agent capabilities. Provides context/guidelines injected into system prompt. Discovered from `skills/` directories.

**Not**: Built-in tool, plugin.

## Sticky Block

Framework-level instruction injected at the head of every system prompt. Content in Go source (not markdown), cannot be overridden by user personality files.

## Tool

Executable capability available to agent. Built-in tools are Go code. Skill tools are markdown-based. Plugins are binary IPC.

**See also**: Built-in tool, skill, plugin.

## Built-in Tool

Tool implemented in Go, compiled into agent binary. Examples: read, write, bash, glob, grep, memory_save, cronjob_create.

**Not**: Skill, plugin.

## Plugin

Binary skill implementing versioned gRPC/IPC contract. Language-agnostic. Loaded at runtime.

**Not**: Built-in tool, skill.

## Context

Information fed to LLM for each call. Includes system prompt (SOUL, IDENTITY, sticky blocks, tools), conversation history, prefill, and memory recall.

## Pruning

Token budget management for conversation history. Triggers: prune oldest turns → summarize → aggressive strip.

## NATS ACL

Per-agent permissions enforced at NATS broker layer. All agents equal: subscribe to own inbox + registry, publish to any agent inbox.

## Control Socket

Unix socket exposed by hub and agent for ctl commands.

| Component | Socket Path | Purpose |
|-----------|------------|---------|
| hub | `$ZLAW_HOME/run/hub.sock` | Agent lifecycle commands |
| agent | `$ZLAW_HOME/agents/<id>/agent.sock` | Local agent control |

ctl connects directly to these sockets. No NATS involved.

## See Also

- [overview.md](./overview.md) — system architecture
- [constraints.md](./constraints.md) — hard rules
- [plans/](../plans/) — implementation plans
