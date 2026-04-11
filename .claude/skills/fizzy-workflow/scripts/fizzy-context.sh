#!/bin/bash
# fizzy-context.sh - Get project context for workflows
# Usage: fizzy-context.sh <board_id>

BOARD_ID="${1:?Usage: fizzy-context.sh <board_id>}"

echo "=== Fizzy Project Context ==="
echo

echo "Columns for board $BOARD_ID:"
fizzy column list --board "$BOARD_ID" | jq -r '.data[] | "  \(.name): \(.id)"'
echo

echo "Your user ID:"
fizzy identity show | jq -r '.accounts[0].user | "  \(.name) (\(.email_address)): \(.id)"'
echo

echo "Git repository:"
git remote get-url origin 2>/dev/null || echo "  Not in a git repository"
