Reconcile the project's current state back into WORKLOG.md and the Fizzy board.

Extra instructions from the user (if any): $ARGUMENTS

## What this does

Looks at the actual codebase and git history, then brings WORKLOG.md and Fizzy into sync with reality. Use this after cards were done outside normal workflow, after a batch of work, or when priorities have shifted.

## Steps

### 1. Gather ground truth

- Read `WORKLOG.md` for the current claimed status of each card
- Run `git log --oneline -30` to see recent commits
- Scan relevant source files to verify which features are actually present

### 2. Reconcile WORKLOG.md

For each card in the table:
- If the code is present but the row still shows `⬜`, flip it to `✅` and set Notes to the relevant merge commit short hash (7 chars)
- If the row shows `✅` but the code is absent, flip back to `⬜` and note the discrepancy
- Reorder rows only if the user's extra instructions ask for it

Editing rules:
- Edit each row in-place with a targeted string replacement — do not reformat or reorder other rows unless instructed
- Use the merge commit hash in Notes, not individual branch commits
- Never commit `WORKLOG.md` — it is gitignored

### 3. Reconcile the Fizzy board

For each card whose status changed in step 2, or where the user's instructions call for action:
- If newly confirmed done: post a completion comment and close the card (`fizzy card close <NUMBER>`)
- If found incomplete/reverted: reopen and post a comment explaining what was found
- If the user requested a priority or order change in `$ARGUMENTS`: comment on the affected cards noting the new priority, and update WORKLOG.md row order accordingly

### 4. Report

Summarise what changed: which cards flipped status, which Fizzy cards were updated, and any discrepancies that need human attention.
