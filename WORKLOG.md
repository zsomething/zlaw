# WORKLOG.md

> **TEMPORARY — do not commit. Listed in .gitignore.**

---

## Implementation Order (Phase 1 — standalone agent)

| # | Card | Status | Notes |
|---|------|--------|-------|
| ✅ | #168 Project bootstrap | done | go.mod, dir skeleton, stubs |
| ✅ | #169 internal/config | done | agent.toml, SOUL.md, IDENTITY.md, fsnotify watcher |
| ✅ | #176 internal/llm/auth | done | TokenSource, StaticKey, OAuth2, credential store |
| ✅ | #170 internal/llm | done | OpenAI-compat client (Minimax + OpenRouter), no Anthropic SDK |
| ✅ | #177 cmd/zlaw-agent auth | done | `auth login/list/remove`; apikey prompt + oauth2 client_credentials; stty echo suppression |
| ✅ | #172 internal/tools | done | Tool registry + executor; builtin current_time; 6 unit tests |
| ✅ | #171 internal/agent | done | ReAct loop, history manager, context builder; 11 unit tests with MockClient |
| ✅ | #174 internal/adapters/cli | done | RunInteractive (REPL), RunOnce, RunStdin; IsTerminal helper |
| ✅ | #173 cmd/zlaw-agent | done | `run` command; signal-based graceful shutdown; REPL vs stdin auto-detect |
| ✅ | #180 builtin tool: Read | done | Read file contents with optional offset/limit |
| ✅ | #181 builtin tool: Write | done | Write content to file, create parent dirs |
| ✅ | #186 builtin tool: Edit | done | Targeted str_replace; fail on ambiguous match |
| ✅ | #204 builtin tool: Glob | done | Find files by glob pattern with ** recursive support |
| ✅ | #205 builtin tool: Grep | done | Regex search over file contents with line numbers |
| ✅ | #206 builtin tool: Bash | done | Run shell commands; timeout (default 30s, max 300s); stdout/stderr/exit code |
| ✅ | #191 internal/agent: durable session history (JSONL store) | done | SessionStore interface + JSONLFileStore; messages persist to $ZLAW_HOME/sessions/<agentName>/ |
| ✅ | #192 internal/llm: retry client | done | RetryClient wrapper; 3 attempts, 500ms base, jitter; honors Retry-After on 429 |
| ✅ | #193 internal/agent: context window pruning | done | pruneMessages helper; char-based token estimate; drop oldest pairs |
| ✅ | #194 internal/agent: context optimization pipeline | done | summarize → prune pipeline; config: context_token_budget, context_summarize_threshold |
| ✅ | #202 introduce ZLAW_HOME env var | done | All runtime paths derived from ZLAW_HOME (defaults to $PWD) |
| ✅ | #203 move credentials to $ZLAW_HOME/credentials.toml | done | ZLAW_CREDENTIALS_FILE override still supported |
| ✅ | #207 derive default agent-dir from $ZLAW_HOME/agents/<name> | done | --agent-dir optional when --agent is set |
| ✅ | #208 move session store to $ZLAW_HOME/sessions/<agentname>/ | done | Replaces old hardcoded path |
| ✅ | #209 add --agent <name> flag | done | Selects $ZLAW_HOME/agents/<name>; replaces required --agent-dir |
| ✅ | #184 builtin tool: WebFetch | done | e0a71e4 |
| ✅ | #185 builtin tool: WebSearch | done | 27fa801 |
| ✅ | #187 builtin tool: HttpRequest | done | 3175814 |
| ✅ | #210 streaming LLM responses | done | f0d9cbd |
| ✅ | #211 enforce tools.allowed allowlist | done | 31e897b |
| ✅ | #212 REPL /clear and /history commands | done | 95b8b9a |
| ✅ | #178 internal/llm: Anthropic native API backend | done | d9b8409 |
| ✅ | #215 concurrent tool execution | done | 153f1f0 |
| ✅ | #217 tool result truncation | done | ad6b197 |
| ✅ | #218 real token counts for optimizer | done | 179d732 |
| ✅ | #222 cascading prune levels | done | fcbb40c |
| ✅ | #219 strip_tool_results prune level | done | fcbb40c (delivered with #222) |
| ✅ | #220 Anthropic prompt caching | done | ef9f847 |
| ✅ | #224 separate model for summarization | done | 19eb91d |
| ✅ | #223 strip_thinking prune level | done | fcbb40c (delivered with #222) |
| ✅ | #221 context prefill at session start | done | 2f88162 |
| ✅ | #230 sticky static context mechanism | done | bc4c83a |
| ✅ | #225 internal/agent: MemoryStore interface + markdown file store | done | ab85069 |
| ✅ | #226 builtin tools: memory_save, memory_recall, memory_delete | done | 16be266 |
| ✅ | #227 inject long-term memories into system prompt | done | 96b28e1 |
| ✅ | #229 built-in sticky context for proactive memory saving | done | 858b3b1 |
| ✅ | #231 CLI-attachable channel support + Telegram adapter | done | 35ed471 |
| ✅ | #175 impl: markdown-based skills system | done | 4d46718 |
| ✅ | #232 reliability: graceful shutdown with in-flight drain | done | 0070a9a |
| ✅ | #189 slash command foundation (channel-agnostic) | done | cc15f0b — SlashCmd registry/dispatcher; /clear /history; CLI + Telegram intercept |
| ✅ | #236 config: static vs dynamic config split (runtime.toml override layer) | done | c2f36e3 — RuntimeConfig struct; Loader merges runtime.toml; WriteRuntimeField |
| ✅ | #241 adapter push interface: outbound delivery without inbound | done | 02eebe1 — Pusher interface + adapter implementations |
| ✅ | #237 polling: periodic check of external services | done | 866df92, 483b48a — CronConfig, cron.toml, background scheduler |
| ✅ | #238 cronjob: scheduled tasks with channel routing | done | 64d5cd0, 3a7fa8d — list_cronjobs/create_cronjob/delete_cronjob tools; wired into serve |
| ✅ | #228 embedding-based semantic memory search | done | 588f7fe, e1fbfb3, a8c93ab — chromem-go vector store; auto-enabled from LLM backend preset |
| ⬜ | #296 Fizzy adapter | partial | adapter type defined in config; credential scaffolding done; adapter implementation pending |

## Phase 2 — Hub + Inter-Agent

| # | Card | Status | Notes |
|---|------|--------|-------|
| ✅ | #243 cmd/zlaw-hub: bootstrap CLI — init, auth, start, status | done | 4d0a19f |
| ✅ | #244 internal/messaging: Messenger interface + NATSMessenger + ChanMessenger | done | 04f4514 |
| ✅ | #245 internal/hub: zlaw.toml config loading + embedded NATS server | done | cae1b36 |
| ✅ | #246 internal/hub: agent supervisor — spawn, stop, restart, restart policy | done | 991c374 |
| ✅ | #247 internal/hub: agent registry — connect, capabilities, heartbeat, deregister | done | 1768fbc |
| ✅ | #248 internal/hub: credential injection at agent spawn time | done | 5b05bf2 |
| ✅ | #249 zlaw-agent: NATS client — connect to hub, heartbeat, inbox subscription | done | 2fa425e |
| ✅ | #250 internal/hub: NATS ACL — per-agent publish/subscribe permissions | done | d9def47 |
| ✅ | #251 A2A: TaskEnvelope format + agent_delegate builtin tool | done | 343c0e8 |
| ✅ | #252 internal/hub: zlaw.hub.inbox — management API handler | done | c86cc8f |
| ✅ | #273 PLAN: decide agent delegation model — manager-only vs peer-to-peer | done | Decision: P2P over NATS. No manager concept. Entry agent = has adapters; Worker = headless P2P. Lifecycle tools CLI-only.
| ✅ | #269 agent config: add roles field to AgentMeta | done | a80fbab |
| ✅ | #270 hub registry: expose agent list over NATS zlaw.registry | done | 3994ea2 — request/reply on zlaw.registry.list |
| ✅ | #271 builtin tools: list_agents / get_agent — readonly, available to all agents | done | 711444e |

| ✅ | #274 nats: enable JetStream in embedded server | 2e58da6 | StoreDir=$ZLAW_HOME/nats |
| ✅ | #275 nats: create durable stream for agent.*.inbox | e81a254, 2e58da6 | AGENT_INBOX stream; WorkQueuePolicy |
| ✅ | #276 nats: migrate agent inbox to JetStream durable pull consumer | e81a254, a4b131b | per-agent durable consumer; depends on #275 |
| ✅ | #291 agent heartbeat mechanism | **P1** | heartbeat infrastructure done (#247, #249); control socket status wiring |
| ✅ | #292 hub control socket server & NATS inbox handler | done | feat/hub-control-socket |
| ⬜ | #293 remove manager concept from hub and ACL | planned | Remove manager flag; simplify ACL; lifecycle tools CLI-only |
| ✅ | #254 zlaw agent CLI: full agent management | **P2** | control socket wired; list/status/stop/restart done |
| ⬜ | #255 internal/hub: audit logger — append-only NATS subscriber | planned | P4 |
| ⬜ | #294 update delegation envelope with source agent and session context | planned | source_agent, session_context fields; sub-agent starts fresh session |
| ⬜ | #256 identity: NKeys keypair generation + hub verification | planned | P4 |
| ⬜ | #257 identity: A2A message signing and verification | planned | P4 |
| ⬜ | #289 interactive CLI/TUI for hub and agent configuration & authentication | future | Bubble Tea TUI for init/auth/agent setup wizard |
| ⬜ | #290 hub web UI dashboard | future | read-only web dashboard (hub status, agent list, logs) |
| ⬜ | #258 isolation: homedir | deferred | P5 |
| ⬜ | #259 isolation: OS user | deferred | P5 |
| ⬜ | #260 isolation: Docker container | deferred | P5 |
| ⬜ | #261 observability: distributed trace IDs across agent hops | deferred | P6 |
| ⬜ | #262 observability: Prometheus metrics endpoint | deferred | P6 |

## Backlog

Items not yet scheduled — Phase 1 scope or deferred pending other work.

| # | Card | Notes |
|---|------|-------|
| ✅ | #266 release: GoReleaser + Docker + Homebrew release pipeline | dfa550c |
| ✅ | #268 agent config: rename name→id; add separate display name field | AgentMeta has ID + Name + DisplayName() fallback |
| ✅ | #267 pretty logs: colored output with per-agent prefix when spawned by hub | cc464a2 — PrettyHandler + supervisor line-prefixer; ZLAW_NO_COLOR opt-out |

---

## Key decisions made this session

- **No Anthropic SDK.** All backends use `openai-go` against OpenAI-compatible endpoints.
- **Backend priority:** Minimax first, OpenRouter second.
- **Auth config is a separate file** (`$ZLAW_HOME/credentials.toml`, 0600).
  Override path via `ZLAW_CREDENTIALS_FILE` env var.
- `agent.toml` references auth by profile name (`auth_profile = "minimax-default"`); no secrets in agent config.
- OAuth2 uses client_credentials grant with in-memory token cache + auto-refresh within 60s of expiry.
- **ZLAW_HOME** is the root for all runtime paths (agents, sessions, credentials). Defaults to `$PWD`.
- **Session persistence** is fully wired: JSONL store under `$ZLAW_HOME/sessions/<agentName>/`.
- **Delegation model: P2P over NATS.** No manager concept. Entry agent = has adapters; Worker = headless P2P. Lifecycle tools (stop/restart) are CLI-only.
- **JetStream always enabled.** No config toggle. Always-on for durable messaging.
- **Adapters are optional.** Agents can be headless (no adapters) for pure P2P worker role.

## Commit trail

| Commit | What |
|--------|------|
| `16094e1` | Bootstrap: go.mod, dir skeleton |
| `972dd74` | internal/config: loading, hot-reload, env expansion |
| `4090a19` | internal/llm: auth layer + OpenAI-compat client (Minimax/OpenRouter) |
| `2c44c47` | internal/agent: durable session history with JSONL file store |
| `896ff9e` | internal/llm: RetryClient with exponential backoff |
| `c53a8c7` | internal/agent: context window pruning |
| `13d9406` | internal/agent: summarize→prune context optimization pipeline |
| `758a65d` | internal/tools: glob builtin |
| `45f86c7` | internal/tools: grep builtin |
| `7463bd8` | internal/tools: bash builtin |
| `68eba32` | internal/config: introduce ZLAW_HOME |
| `df7c8b0` | chore: gitignore credentials.toml and sessions/ under ZLAW_HOME |
| `b1dbd95` | internal/agent: persist session metadata alongside JSONL history |
| `343c0e8` | internal/tools: A2A TaskEnvelope format + agent_delegate builtin tool |
| `d9def47` | internal/hub: NATS ACL — per-agent publish/subscribe permissions |
| `5b05bf2` | internal/hub: inject agent credentials as env vars at spawn time |
| `1768fbc` | internal/hub: agent registry — subscribe, heartbeat, deregister |
| `2fa425e` | internal/agent: NATS hub client — register, heartbeat, inbox task handling |
| `cae1b36` | internal/hub: embed NATS server and wire hub start command |
| `991c374` | internal/hub: agent supervisor — spawn, stop, restart, restart policy |
| `04f4514` | internal/messaging: Messenger interface + NATSMessenger + ChanMessenger |
| `4d0a19f` | cmd/zlaw: bootstrap CLI — init, auth, start, status |
| `dfa550c` | release: GoReleaser + Docker + Homebrew release pipeline |
| `a80fbab` | feat(config): add roles field to AgentMeta |
| `3994ea2` | feat(hub/registry): expose agent list over NATS request/reply |
| `711444e` | feat(builtin): add list_agents and get_agent discovery tools |
| `cc464a2` | feat(logging): unified pretty log format across hub and agents |
| `6f79655` | feat(agent logs): add zlaw agent logs command for streaming agent logs |
| `37dbace` | refactor: separate agent configs from workspace, per-agent credentials |
| `288e02e` | **Merge** config refactor: separate agent configs from workspace |
| `41711f9` | feat(logging): unified pretty log format across hub and agents |
| `1f659cf` | fix: address golangci-lint issues |
| `b1887dc` | feat(hub): control socket server + NATS inbox handler (#292) |
| `bc5b56d` | **Merge** feat/hub-control-socket: hub control socket + NATS inbox handler (#292) |
| `b641e54` | fix: remove unused init templates + add golangci-lint to pre-commit hook |
| `4771a71` | feat(hub): wire heartbeat status into control socket — AgentInfo merges supervisor + registry state; update agent CLI for conn/heartbeat display (#291, #254) |
| `3ff5173` | feat(builtin): add agent_stop and agent_restart tools, rename discovery tools (#253) |
| `f3e8e39` | docs: update README with multi-agent features and current config |
| `0e788c9` | docs: add README.media clarifying AI-generated image licenses |
| `9c15c1a` | docs: add zlaw logo to README |
| `91afeff` | docs: highlight multi-agent in tagline |
| `9f9c7ab` | docs: update README to reflect current zlaw.toml and agent.toml config |
| `39dce52` | docs: adjust goals/non-goals to avoid overpromising |
| `a4b131b` | fix(agent): use FetchOnSubject for durable pull consumers with filter |
| `2e58da6` | chore(hub): always enable JetStream |
| `38184f5` | refactor(config): simplify adapter config to use array directly |
