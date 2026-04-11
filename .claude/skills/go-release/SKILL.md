---
name: go-release
description: Set up automated releases for Go CLI apps using GoReleaser, GitHub Actions, GHCR (Docker), and Homebrew tap. Use when the user wants to add release automation, Dockerfile, or Homebrew distribution to a Go project.
compatibility: Requires go and git in PATH. Project must have a go.mod file.
metadata:
  {
    "author": "akhy",
    "version": "1.0.0",
    "openclaw":
      {
        "emoji": "🚀",
        "homepage": "https://goreleaser.com",
        "keywords": ["goreleaser", "github releases", "ghcr", "homebrew", "docker", "release", "go", "golang", "binary distribution"],
        "requires": { "bins": ["go", "git"] },
        "install":
          [
            {
              "id": "brew-goreleaser",
              "kind": "brew",
              "formula": "goreleaser",
              "bins": ["goreleaser"],
              "label": "Install goreleaser (brew)",
            },
          ],
      },
  }
---

# Go Release Setup Skill

Sets up release infrastructure for Go CLI apps: GoReleaser config, Dockerfile, and GitHub Actions workflows.

## Step 1 — Gather Project Info

Read `go.mod` to infer:
- **Module path** (e.g. `github.com/owner/repo`)
- **Go version** — use the version from the `go` directive in `go.mod` if present; if `go.mod` doesn't exist or has no `go` directive, fetch the current latest stable Go version from `https://go.dev/VERSION?m=text` and use that
- **Binary name** — check `cmd/` subdirectories; if multiple, ask which one(s) to release; if absent, use the last segment of the module path

Derive `<github-owner>` and `<github-repo>` from the module path.

If any of the following cannot be inferred, ask the user:
- **CGO required?** — scan source files for `import "C"`; if found, CGO = true, else CGO = false
- **App description** — one-line description for the Homebrew formula and release header
- **License** — check `LICENSE` file; if absent, ask (default: MIT)
- **Distribution channels** to enable (ask the user; suggest all as default):
  - `github` — GitHub Releases binary archives + checksums (always required)
  - `docker` — Dockerfile + GHCR image push via `docker.yml` workflow
  - `homebrew` — Homebrew formula pushed to `<github-owner>/homebrew-tap` repo
- **Homebrew tap repo** *(homebrew only)* — default `<github-owner>/homebrew-tap`

## Step 2 — Check What Already Exists

Before generating each file, check if it exists. Skip or ask to confirm overwrite:
- `Dockerfile`
- `.goreleaser.yaml`
- `.github/workflows/release.yml`
- `.github/workflows/docker.yml`

## Step 3 — Generate `.goreleaser.yaml`

Use `assets/goreleaser.yaml` as the template. Substitute all `<placeholder>` values. Then:
- Set `CGO_ENABLED=1` and remove `windows` from `goos` if CGO is required
- Remove the `brews` section if `homebrew` is not enabled
- Remove the Docker `**Docker:**` block from `release.header` if `docker` is not enabled
- Remove the Homebrew `**Homebrew:**` block from `release.header` if `homebrew` is not enabled
- Remove `TAP_GITHUB_TOKEN` from the env block in `release.yml` if `homebrew` is not enabled

## Step 4 — Generate `Dockerfile` *(only if `docker` channel enabled)*

- No CGO: use `assets/Dockerfile.alpine` as template
- CGO required: use `assets/Dockerfile.debian` as template

Substitute `<go-version>`, `<module-path>`, and `<binary-name>`. Adjust `main` path if the project doesn't use `cmd/<binary-name>` layout.

## Step 5 — Generate GitHub Workflows

Create `.github/workflows/` if it doesn't exist.

| Workflow | Source template | When to generate |
|---|---|---|
| `release.yml` | `assets/workflows/release.yml` | Always (`github` channel) |
| `docker.yml` | `assets/workflows/docker.yml` | Only if `docker` channel enabled |

Substitute `<go-version>` in each workflow. For `release.yml`, remove the `TAP_GITHUB_TOKEN` env line if `homebrew` is not enabled.

> **Note:** CI workflows (tests, lint) are handled by the `go-ci` skill. Only add them here if the user hasn't already set up `go-ci`.

## Step 6 — Optional: Create `internal/version` Package

If no version package exists and the project's ldflags reference `internal/version`, create `internal/version/version.go`:

```go
package version

var (
    Version   = "dev"
    GitCommit = "unknown"
    BuildDate = "unknown"
)
```

## Step 7 — Post-Setup Instructions

Tell the user:

1. **GitHub Secrets required:**
   - `GITHUB_TOKEN` — auto-provided by GitHub Actions
   - `TAP_GITHUB_TOKEN` *(homebrew only)* — GitHub PAT with `repo` scope for the tap repo

2. **Homebrew tap repo** *(homebrew only)* — create `<github-owner>/homebrew-tap` with a `Formula/` directory if it doesn't exist

3. **First release:**
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. **Test locally (snapshot, no publish):**
   ```bash
   goreleaser release --snapshot --clean
   ```
