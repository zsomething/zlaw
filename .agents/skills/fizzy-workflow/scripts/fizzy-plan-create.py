#!/usr/bin/env python3
# /// script
# requires-python = ">=3.8"
# ///
"""
Create Fizzy cards from a JSON plan file.
Usage: python3 fizzy-plan-create.py <plan.json> [--dry-run]
"""

import argparse
import json
import subprocess
import sys


def run(cmd):
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or f"command failed: {' '.join(cmd)}")
    return result.stdout.strip()


def fail(msg, partial=None):
    out = {"ok": False, "error": msg}
    if partial:
        out["partial"] = partial
    print(json.dumps(out))
    sys.exit(1)


def log(msg):
    print(msg, file=sys.stderr)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("plan", help="JSON plan file")
    parser.add_argument("--dry-run", action="store_true", help="Print what would be created without calling the API")
    args = parser.parse_args()

    try:
        with open(args.plan) as f:
            plan = json.load(f)
    except (OSError, json.JSONDecodeError) as e:
        fail(str(e))

    board_id = plan.get("board_id", "").strip()
    if not board_id:
        fail("missing board_id in plan file")

    cards = plan.get("cards") or []
    if not cards:
        fail("no cards defined in plan file")

    created = []

    for i, card in enumerate(cards):
        title = card.get("title", "").strip()
        if not title:
            fail(f"cards[{i}] missing title")

        description = card.get("description", "")
        tags = card.get("tags") or []
        steps = card.get("steps") or []

        if args.dry_run:
            created.append({"title": title, "tags": tags, "steps": steps})
            log(f"[dry-run] {title!r} (tags: {tags}, {len(steps)} steps)")
            continue

        cmd = ["fizzy", "card", "create", "--board", board_id, "--title", title]
        if description:
            cmd += ["--description", description]

        try:
            data = json.loads(run(cmd))
            number = data["data"]["number"]
        except (RuntimeError, json.JSONDecodeError, KeyError) as e:
            fail(f"card create failed: {e}")

        for tag in tags:
            try:
                run(["fizzy", "card", "tag", str(number), "--tag", tag])
            except RuntimeError as e:
                fail(f"tag failed: {e}", partial={"number": number, "title": title, "steps_created": 0})

        steps_created = 0
        for step in steps:
            try:
                run(["fizzy", "step", "create", "--card", str(number), "--content", step])
                steps_created += 1
            except RuntimeError as e:
                fail(f"step create failed: {e}", partial={"number": number, "title": title, "steps_created": steps_created})

        created.append({"number": number, "title": title, "tags": tags, "steps_created": steps_created})
        log(f"#{number} {title!r} (tags: {tags}, {steps_created} steps)")

    print(json.dumps({"ok": True, "dry_run": args.dry_run, "created": created}))


if __name__ == "__main__":
    main()
