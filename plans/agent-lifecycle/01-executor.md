# Phase 1: Executor Abstraction

## Goal

Create executor abstraction for spawning and managing agent processes.

## Completed ✅

### 1.1 Executor Interface

**File:** `internal/executor/executor.go`

```go
type Executor interface {
    Start(ctx context.Context, cfg AgentConfig) error
    Stop(ctx context.Context, id string) error
    Status(ctx context.Context, id string) (Status, error)
    Logs(ctx context.Context, id string) (io.ReadCloser, error)
}

type AgentConfig struct {
    ID            string
    Dir           string
    Binary        string
    Executor      string // "subprocess", "systemd", "docker"
    Target        string // "local", "ssh"
    TargetSSH     string
    RestartPolicy string // "always", "on-failure", "never"
    NATSURL       string
}

type Status struct {
    ID      string
    Running bool
    PID     int
    Error   error
}
```

### 1.2 SubprocessExecutor

**File:** `internal/executor/subprocess.go`

- Spawns agents as child processes
- Self-monitoring with exponential backoff restart
- Supports restart_policy (always/on-failure/never)

### 1.3 AgentEntry Fields

**File:** `internal/config/hub.go`

Added fields:
- `Executor string` — "subprocess", "systemd", "docker"
- `Target string` — "local", "ssh"
- `TargetSSH string` — SSH connection string

### 1.4 Stubs

**Files:**
- `internal/executor/systemd.go` — SystemdExecutor (placeholder)
- `internal/executor/docker.go` — DockerExecutor (placeholder)

## Not Implemented ❌

### 1.5 Credential Injection

SubprocessExecutor needs to inject credentials similar to hub's supervisor:
- Read `$ZLAW_HOME/credentials.toml`
- Write filtered profiles to `$ZLAW_HOME/run/credentials/<id>.toml`
- Set `ZLAW_CREDENTIALS_FILE` env var

### 1.6 SystemdExecutor Full Implementation

Need to implement:
- Create systemd unit file
- `systemctl start/stop` for lifecycle
- `journalctl` for logs

### 1.7 NATS URL Injection

Currently SubprocessExecutor sets `ZLAW_NATS_URL` but needs to connect to hub's NATS, not its own.

## Dependencies

Phase 1 is self-contained. No dependencies on other phases.
