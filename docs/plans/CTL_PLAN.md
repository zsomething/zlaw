# Plan: `zlaw ctl` subcommand

## Analogy

| Kubernetes | zlaw |
|---|---|
| pod | agent (atomic deployment unit) |
| deployment controller | hub |
| kubectl | ctl |

---

## Command tree

```
zlaw ctl get agents [--json]
zlaw ctl get agent <id> [--json]
zlaw ctl get hub [--json]
zlaw ctl stop <id>
zlaw ctl restart <id>
zlaw ctl disable <id>
zlaw ctl enable <id>
zlaw ctl delete <id>
zlaw ctl create agent <id> [--workspace PATH]
zlaw ctl configure <id> <key> <value>
zlaw ctl logs <id> [--level L] [--since D] [--follow]
zlaw ctl top                                             # TUI, later
```

All ctl commands talk to running hub via Unix socket (`$ZLAW_HOME/control.sock`).

---

## Responsibility split

### `agent` (single-agent, dev-time — keeps these):
- `agent run`    — interactive REPL
- `agent serve`  — daemon mode
- `agent attach` — attach to daemon
- `agent logs`   — stream logs via NATS (dev use, no hub needed)
- `agent auth`   — manage per-agent credentials

### `hub` (hub lifecycle — keeps these):
- `hub start/run/stop/restart/status`

### `ctl` (operational, hub-connected — new):
All commands below move from `agent` to `ctl`:
- `agent list`      → `ctl get agents`
- `agent status`    → `ctl get agent <id>`
- `agent create`    → `ctl create agent <id>`
- `agent configure` → `ctl configure <id> <key> <val>`
- `agent disable`   → `ctl disable <id>`
- `agent enable`    → `ctl enable <id>`
- `agent delete`    → `ctl delete <id>`
- `agent stop`      → `ctl stop <id>`
- `agent restart`   → `ctl restart <id>`

New in `ctl` (not in old `agent`):
- `ctl get hub`    — hub status (mirrors `hub status`, read via socket)
- `ctl logs`       — stream hub-managed agent logs (NATS, same as `agent logs` but id-first)
- `ctl top`        — live TUI overview (bubbletea, deferred)

---

## File changes

### New
- `cmd/zlaw/ctl.go` — `CtlCmd` + all subcommands + socket helpers (moved from agent.go)

### Modified
- `cmd/zlaw/agent.go` — remove: List/Status/Create/Configure/Disable/Enable/Delete/Stop/Restart cmds + socket helpers
- `cmd/zlaw/main.go` — add `Ctl CtlCmd`

### Deleted
Nothing deleted; agent.go just shrinks.

---

## `ctl.go` struct layout

```go
type CtlCmd struct {
    Get       CtlGetCmd       `cmd:"" help:"get resource info"`
    Stop      CtlStopCmd      `cmd:"" help:"stop an agent"`
    Restart   CtlRestartCmd   `cmd:"" help:"restart an agent"`
    Disable   CtlDisableCmd   `cmd:"" help:"disable an agent (stop + prevent respawn)"`
    Enable    CtlEnableCmd    `cmd:"" help:"re-enable a disabled agent"`
    Delete    CtlDeleteCmd    `cmd:"" help:"stop and remove an agent"`
    Create    CtlCreateCmd    `cmd:"" help:"create a resource"`
    Configure CtlConfigureCmd `cmd:"" help:"update a runtime field"`
    Logs      CtlLogsCmd      `cmd:"" help:"stream agent logs"`
    Top       CtlTopCmd       `cmd:"" help:"live agent overview (TUI)"`
}

type CtlGetCmd struct {
    Agents  CtlGetAgentsCmd `cmd:"" help:"list all agents"`
    Agent   CtlGetAgentCmd  `cmd:"" help:"show agent detail"`
    Hub     CtlGetHubCmd    `cmd:"" help:"show hub status"`
}

type CtlCreateCmd struct {
    Agent CtlCreateAgentCmd `cmd:"" help:"create and spawn a new agent"`
}
```

---

## Output format

`ctl get agents` table (default):
```
NAME       CONN         HEARTBEAT     ROLES
alice     connected    14:23:01      [coder]
bob       disconnected —             []
```

`ctl get agent <id>`:
```
ID:        manager
Running:   yes
PID:       12345
Conn:      connected
Heartbeat: 14:23:01
Caps:      bash, read, write, ...
Roles:     manager
```

`ctl get hub`:
```
Hub:       main
NATS:      nats://127.0.0.1:4222
JetStream: yes
Agents:    2
Status:    running
```

All commands support `--json`.

---

## Implementation order (priority: CLI before TUI)

1. `ctl.go` skeleton + all non-TUI commands
2. Remove migrated commands from `agent.go`
3. Register in `main.go`
4. `ctl top` TUI (bubbletea, separate step)

---

## Notes

- `ctl logs` reuses `AgentLogsCmd` logic — same NATS subscription, just moved
- Socket helpers (`socketConn`, `agentAction`, etc.) move wholesale to `ctl.go`
- `hub status` stays in `hub` (hub lifecycle); `ctl get hub` is new but reads same data
- No breaking change to `hub` commands
- `agent` operational commands disappear — users migrate to `ctl`
