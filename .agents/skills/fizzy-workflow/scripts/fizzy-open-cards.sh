#!/bin/bash
# fizzy-open-cards.sh - List open cards on a board
# Usage: fizzy-open-cards.sh <board_id>

BOARD_ID="${1:?Usage: fizzy-open-cards.sh <board_id>}"

# Use fizzy's built-in --jq flag (unreleased); fall back to external jq
fizzy_jq() {
    local filter="$1"; shift
    local out rc
    out=$(fizzy "$@" --jq "$filter" 2>&1)
    rc=$?
    if [ $rc -eq 0 ]; then
        echo "$out"
    elif echo "$out" | grep -qi "unknown flag\|flag provided but not defined"; then
        fizzy "$@" | jq "$filter"
    else
        echo "$out" >&2
        return $rc
    fi
}

fizzy_jq '[.data[] | {number, title, assignees: [.assignees[].name]}]' card list --board "$BOARD_ID" --all
