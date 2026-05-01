# Refactor Plan: Architecture Alignment

## Goal

Align current implementation with the architecture defined in `docs/design/`. Eliminate Hub's access to agent filesystem, enforce absolute path requirements, and complete the identity system.

## Gaps Identified

| # | Gap | Severity | Files |
|---|-----|----------|-------|
| G1 | Hub reads `agent.toml` to discover auth profiles | HIGH | `internal/hub/credentials.go` |
| G2 | Hub reads `credentials.toml` from agent dir | HIGH | `internal/hub/credentials.go` |
| G3 | Hub uses `ZlawHome()` as fallback for agent dirs | HIGH | `internal/hub/credentials.go`, `internal/hub/supervisor.go` |
| G4 | Hub writes `runtime.toml` to agent directories | HIGH | `internal/hub/control.go` |
| G5 | Identity package is empty stub | HIGH | `internal/identity/identity.go` |
| G6 | Subprocess credential filtering not implemented | MED | `internal/tools/builtin/bash.go` |
| G7 | Agent config uses `ZlawHome()` for fallback | LOW | `internal/app/agent_serve.go` |

## Implementation Order

```
Phase 1 (Foundation): Remove Hub filesystem access
  ↓
Phase 2 (Credential Model): Agent-provided auth profiles
  ↓
Phase 3 (Identity): Complete stub identity package
  ↓
Phase 4 (Isolation): Subprocess credential filtering
  ↓
Phase 5 (Polish): Remove remaining ZlawHome() fallbacks
```

## Phase Details

| Phase | Plan | Focus |
|-------|------|-------|
| 1 | `01-hub-fs-isolation.md` | Stop Hub from reading/writing agent directories |
| 2 | `02-auth-profile-registration.md` | Agent provides auth profiles at NATS registration |
| 3 | `03-identity-system.md` | Complete keypair generation, verification, message signing |
| 4 | `04-subprocess-filtering.md` | Filter credential env vars from subprocess execution |
| 5 | `05-absolute-paths.md` | Remove all ZlawHome() fallbacks, require absolute paths |

## Non-Goals

- Web UI, TUI
- Sandboxing (isolation levels in design, but not the focus here)
- Plugin binary contract (deferred)
- Working memory (per-session scratch state)

## See Also

- `docs/design/constraints.md` — hard rules
- `docs/design/security.md` — security model
- `docs/design/agent_credentials.md` — credential injection design
- `plans/archive/separation.md` — previous violation analysis