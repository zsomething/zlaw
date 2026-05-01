# Agent Portability: Consolidate Per-Agent Files Under `ZLAW_AGENT_HOME`

## Problem

Per-agent files are scattered across five top-level directories in `$ZLAW_HOME`. Additionally, the hub currently knows too much about agent internals — it writes SOUL.md, IDENTITY.md, creates workspace directories, and reads `agent.toml` for `disabled` state. This couples hub and agent tightly, making agents non-portable.

## Design Principles (Kubernetes Analogy)

```
Kubernetes          zlaw
─────────────────── ──────────────────────────────
Control Plane    ↔  Hub (zlaw hub)
Node             ↔  Agent (zlaw agent serve)
kubectl          ↔  ctl (zlaw ctl)
Pod Spec (etcd)  ↔  zlaw.toml [[agents]] registry
ZLAW_AGENT_HOME  ↔  Container filesystem root
~/.kube/         ↔  ZLAW_HOME (ctl-owned convention)
```

**`ZLAW_HOME` is a `ctl`-owned convention**, not a hub concept. Hub doesn't need `ZLAW_HOME` at runtime — it needs only:
1. A config file path (`--config`, defaults to `$ZLAW_HOME/zlaw.toml` when started via ctl)
2. A run dir for sockets/PIDs/injected-creds (`--run-dir`, defaults to `$ZLAW_HOME/run/` when started via ctl)

All `AgentEntry.Dir` paths in `zlaw.toml` must be **absolute** — written by ctl at create time. Hub's supervisor never calls `ZlawHome()` to resolve agent paths.

**Hub knows only:**
- Agent name
- `ZLAW_AGENT_HOME` (absolute path — the `dir` field in `zlaw.toml`)
- Binary to run (optional)
- Restart policy
- Disabled flag (in `zlaw.toml` registry, not `agent.toml`)

**Hub reads at spawn (defined interface):**
- `$ZLAW_AGENT_HOME/agent.toml` — to discover which credential profiles the agent needs
- `$ZLAW_AGENT_HOME/credentials.toml` — credential source, generates runtime credentials file

**Hub never:**
- Uses `ZLAW_HOME` at runtime (only at startup via ctl-provided `--config`/`--run-dir`)
- Creates SOUL.md, IDENTITY.md
- Creates workspace/, sessions/, memories/
- Writes inside `ZLAW_AGENT_HOME`
- Resolves relative paths using `ZlawHome()`

**`ctl` (human-operated) is the bootstrapper and `ZLAW_HOME` owner:**
- Owns `ZLAW_HOME` directory layout
- Creates `ZLAW_AGENT_HOME/` structure and scaffolds all agent files
- Starts hub with explicit `--config` and `--run-dir` paths
- Registers agents with hub via control socket (absolute `dir` path)
- Manages lifecycle via hub control socket

## Target Directory Layout

```
$ZLAW_HOME/                (ctl-owned convention for local setups)
  zlaw.toml                (hub config + agent registry — written by ctl/init)
  credentials.toml         (global credential fallback — user-maintained)
  skills/                  (global shared skills — user-maintained)
  run/                     (hub runtime: sockets, PIDs, injected creds — ephemeral, hub-written)
  nats/                    (JetStream store — hub-written, path from zlaw.toml NATS.StoreDir)
  agents/<id>/           (= ZLAW_AGENT_HOME for local agents, ctl-created)

$ZLAW_AGENT_HOME/          (agent's self-contained root — can live anywhere)
  agent.toml               (was: agents/<id>/agent.toml)
  runtime.toml             (was: agents/<id>/runtime.toml)
  credentials.toml         (was: agents/<id>/credentials.toml)
  cron.toml                (was: agents/<id>/cron.toml)
  SOUL.md                  (was: agents/<id>/SOUL.md)
  IDENTITY.md              (was: agents/<id>/IDENTITY.md)
  skills/                  (was: agents/<id>/skills/)
  sessions/                (was: agents/<id>/sessions/)
  memories/                (was: agents/<id>/memories/)
  workspace/               (was: agents/<id>/workspace/ — agent's writable CWD for file tools)
```

## `AgentEntry` Changes (`internal/config/hub.go`)

```go
// AgentEntry describes a single agent supervised by the hub.
// Hub knows only what it needs to manage the process lifecycle.
type AgentEntry struct {
    Name          string        `toml:"name"`
    Dir           string        `toml:"dir"`            // = ZLAW_AGENT_HOME
    Binary        string        `toml:"binary"`         // optional
    RestartPolicy RestartPolicy `toml:"restart_policy"`
    Disabled      bool          `toml:"disabled"`       // moved from agent.toml
}
```

Removed: `Workspace string` — hub doesn't need this. Agent's workspace is internal.

`Disabled` moves from `agent.toml` to `zlaw.toml` registry so hub can manage it without touching agent files. Remove `config.WriteAgentDisabled()`.

## New: `config.AgentHome()` (`internal/config/home.go`)

```go
// AgentHome returns the agent's root directory from ZLAW_AGENT_HOME.
// All per-agent files are relative to this path.
// The agent process does not need ZLAW_HOME to find its own files.
func AgentHome() string {
    return os.Getenv("ZLAW_AGENT_HOME")
}
```

Hub sets `ZLAW_AGENT_HOME` when spawning the agent process. For remote/container agents, the runtime environment sets it.

## Code Changes

### Agent-side path functions

| File | Change |
|------|--------|
| `internal/config/home.go` | Add `AgentHome()` |
| `internal/agent/history.go:189` | `SessionDir()` → `filepath.Join(AgentHome(), "sessions")` |
| `internal/agent/memory.go:41` | `MemoryDir()` → `filepath.Join(AgentHome(), "memories")` |
| `internal/config/config.go:~481` | Personality dir from `AgentHome()`, not workspace |

### Hub changes (remove agent-internal knowledge + ZLAW_HOME dependency)

| File | Change |
|------|--------|
| `internal/config/hub.go:57` | Remove `Workspace` field from `AgentEntry` |
| `internal/config/hub.go:149` | Remove `Workspace` from `AddAgent` serialization |
| `internal/config/hub.go:186` | Remove `WriteAgentDisabled()` — hub updates `zlaw.toml` registry instead |
| `internal/config/hub.go` | Add `Disabled bool` to `AgentEntry`, add `SetAgentDisabled(name, bool)` that writes zlaw.toml |
| `internal/hub/credentials.go:23` | Remove `resolveWorkspaceDir()` entirely |
| `internal/hub/inbox.go:56-64` | Remove `scaffoldSoulMD`, `scaffoldIdentityMD` constants |
| `internal/hub/inbox.go:221` | Remove `workspaceDir` param from `AgentCreate()` |
| `internal/hub/inbox.go:229` | `opAgentCreate` → `opAgentRegister`: validate `dir` exists, register, spawn. No file creation. |
| `internal/hub/supervisor.go:382` | Remove `ZlawHome()` path resolution — `AgentEntry.Dir` is always absolute; inject `ZLAW_AGENT_HOME=entry.Dir` |
| `internal/hub/supervisor.go` | Remove `ZLAW_HOME` from injected env vars |
| `internal/hub/control.go` | `disable/enable`: call `SetAgentDisabled()` (writes zlaw.toml, not agent.toml) |
| `internal/app/hub_daemon.go` | Accept `--run-dir` flag; pass run dir explicitly instead of deriving from `ZlawHome()` |
| `cmd/zlaw/hub.go` (or hub CLI) | When starting hub, pass `--config $ZLAW_HOME/zlaw.toml --run-dir $ZLAW_HOME/run` |

### ctl subcommand (new — see ctl_plan.md)

`ctl create agent <id>` owns all scaffolding:
1. Create `$ZLAW_HOME/agents/<id>/` (or custom path)
2. Write `agent.toml` template (from ctl's templates)
3. Write `credentials.toml` template
4. Write `SOUL.md`, `IDENTITY.md`
5. Create `workspace/` subdir
6. Send `agent.register` to hub via control socket (name + dir + restart_policy)
7. Optionally send `ctl start <id>` to spawn it

Scaffold templates move from `inbox.go` constants → `cmd/zlaw/ctl.go` (ctl's responsibility).

### CLI flags cleanup

| File | Change |
|------|--------|
| `cmd/zlaw/agent.go:62` | `resolveWorkspace()` fallback → `filepath.Join(AgentHome(), "workspace")` |
| `cmd/zlaw/init.go:64,117` | `initWorkspace`/`initAgent` → write to `agents/<id>/` root, create `workspace/` subdir |

## Data Flow After Change

**Local agent "alice" created via ctl:**
```
ctl create agent alice
  → creates $ZLAW_HOME/agents/alice/
  → scaffolds agent.toml, credentials.toml, SOUL.md, IDENTITY.md, workspace/
  → sends agent.register{name:"alice", dir:"$ZLAW_HOME/agents/alice"} to hub
  → hub registers in zlaw.toml, spawns with ZLAW_AGENT_HOME=$ZLAW_HOME/agents/alice
```

**Agent process startup:**
```
ZLAW_AGENT_HOME=$ZLAW_HOME/agents/alice
  → config.AgentHome() = $ZLAW_HOME/agents/alice
  → loadPersonality(AgentHome()) → reads SOUL.md, IDENTITY.md ✓
  → SessionDir() = $ZLAW_HOME/agents/alice/sessions/ ✓
  → MemoryDir() = $ZLAW_HOME/agents/alice/memories/ ✓
  → workspace = $ZLAW_HOME/agents/alice/workspace/ ✓
```

**Hub supervisor — what it injects:**
```
env vars set at spawn:
  ZLAW_AGENT_HOME=/absolute/path/to/agents/alice   (from AgentEntry.Dir — always absolute)
  ZLAW_CREDENTIALS_FILE=<run-dir>/credentials/alice.toml  (hub-generated, ephemeral)
  NATS_URL=nats://127.0.0.1:4222
```

Hub reads `agent.toml` + `credentials.toml` from `ZLAW_AGENT_HOME` once at spawn to generate the runtime credentials file. No `ZLAW_HOME` injected — agent doesn't need it.

**Remote/container agent (future):**
```
Container env sets ZLAW_AGENT_HOME=/container/path/alice
Agent binary reads same env var → all paths resolve correctly
Hub connects via NATS — doesn't know or care about internal file layout
```

## What Stays in Hub vs. Moves to ctl

| Operation | Before | After |
|-----------|--------|-------|
| Create agent dir | hub (`opAgentCreate`) | ctl |
| Scaffold agent.toml | hub | ctl |
| Scaffold credentials.toml | hub | ctl |
| Scaffold SOUL.md, IDENTITY.md | hub | ctl |
| Create workspace/ | hub | ctl |
| Register in zlaw.toml | hub | hub (via ctl request) |
| Spawn process | hub | hub (via ctl request) |
| Read credentials at spawn | hub (defined interface) | hub (defined interface) |
| disable/enable flag | writes agent.toml (hub) | writes zlaw.toml registry (hub) |

## Backward Compatibility

- Agents with explicit `workspace = "..."` in old `zlaw.toml` entries: `Workspace` field removed — migrate by setting `ZLAW_WORKSPACE` env var or updating agent to read from `ZLAW_AGENT_HOME` directly
- `agent.toml` `disabled` field: still read on startup for backward compat, but hub writes disabled state to `zlaw.toml` registry going forward
- No data migration: existing scattered directories stay in place; new agents use new layout

## Verification

```sh
export ZLAW_HOME=/tmp/zlaw-test && rm -rf $ZLAW_HOME
zlaw hub start &

# New agent via ctl
zlaw ctl create agent testbot
# Expect: agents/testbot/{agent.toml,credentials.toml,SOUL.md,IDENTITY.md,workspace/}
# Expect: NO workspaces/testbot/, NO sessions/testbot/, NO memories/testbot/

# Verify hub only knows name + dir + restart_policy
grep testbot $ZLAW_HOME/zlaw.toml
# Expect: name="testbot", dir="...", NO workspace field

# Run agent
ZLAW_AGENT_HOME=$ZLAW_HOME/agents/testbot zlaw agent run --agent testbot
# After use: agents/testbot/sessions/ and agents/testbot/memories/ populated

# Disable via ctl — updates zlaw.toml, NOT agent.toml
zlaw ctl disable testbot
grep disabled $ZLAW_HOME/zlaw.toml  # should appear here
grep disabled $ZLAW_HOME/agents/testbot/agent.toml  # should NOT appear here

go build ./...
go test ./internal/agent/... ./internal/config/... ./internal/hub/... ./internal/app/...
```
