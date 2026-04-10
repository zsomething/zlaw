#!/usr/bin/env bash
# serve-local.sh — build and run the agent daemon for local testing.
#
# Reads secrets from .env (gitignored) so they are never embedded in code or
# sent to any external service. The script exports them as env vars that the
# process inherits; they do not appear in config files or LLM calls.
#
# Usage:
#   ./scripts/serve-local.sh [--agent <name>]   # default: "default"
#   ZLAW_LOG_LEVEL=debug ./scripts/serve-local.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

AGENT="${1:-default}"
# Allow --agent <name> flag form too.
if [[ "$AGENT" == "--agent" && -n "${2:-}" ]]; then
  AGENT="$2"
fi

# ── Load .env (secrets stay in shell, never written anywhere) ────────────────
ENV_FILE="$REPO_ROOT/.env"
if [[ -f "$ENV_FILE" ]]; then
  # Export only lines of the form KEY=VALUE; skip comments and blanks.
  set -o allexport
  # shellcheck source=/dev/null
  source "$ENV_FILE"
  set +o allexport
  echo "[serve-local] loaded $ENV_FILE"
else
  echo "[serve-local] warning: $ENV_FILE not found — Telegram adapter will be disabled"
fi

# ── Set ZLAW_HOME (local dev uses repo root to keep data self-contained) ─────
export ZLAW_HOME="${ZLAW_HOME:-$REPO_ROOT}"

# ── Validate required credentials ────────────────────────────────────────────
CREDS="$ZLAW_HOME/credentials.toml"
if [[ ! -f "$CREDS" ]]; then
  cat >&2 <<EOF
[serve-local] error: credentials.toml not found at $CREDS.

Run the auth setup first:
  ZLAW_HOME=$ZLAW_HOME ./zlaw-agent auth login

This creates credentials.toml with your LLM API key.
EOF
  exit 1
fi

# ── Build ────────────────────────────────────────────────────────────────────
echo "[serve-local] building..."
go build -o "$REPO_ROOT/zlaw-agent" ./cmd/zlaw-agent
echo "[serve-local] build ok"

echo "[serve-local] starting daemon (agent=$AGENT, ZLAW_HOME=$ZLAW_HOME)"
exec "$REPO_ROOT/zlaw-agent" --agent "$AGENT" serve
