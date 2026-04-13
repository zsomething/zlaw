Pick the next card from WORKLOG.md and implement it end-to-end.

## Steps

1. Read `WORKLOG.md` and find the first row with status `⬜` (todo). That is the target card. Note its card number and title.

2. **Start the card on Fizzy** using the fizzy-workflow skill:
   - Get card details: `fizzy card show <NUMBER>`
   - Find the "Doing" column if one exists: `fizzy column list --board <BOARD_ID>`
   - Move to Doing and assign to self if possible

3. **Create a feature branch** before writing any code:
   - Derive a short slug from the card title: 2–4 lowercase words, hyphen-separated (e.g. `telegram-adapter`)
   - `git checkout -b feat/<short-slug>`
   - Never include card numbers or tool references in the branch name

4. **Implement the card** fully. Follow all coding conventions in CLAUDE.md:
   - Interfaces first, explicit error handling, context propagation
   - Tests alongside code
   - Register any new tools in `cmd/zlaw-agent/cmd_run.go`
   - Run `go test ./...` before committing — all tests must pass

5. **Commit** to the feature branch using the `/commit` skill.

6. **Merge to main** with no fast-forward:
   ```
   git checkout main
   git merge --no-ff feat/<short-slug> -m "feat(<scope>): <description>"
   git branch -d feat/<short-slug>
   ```
   The merge commit message follows conventional commits — no card numbers or external references.

7. **Close the card on Fizzy**:
   - Post a completion comment with the merge commit hash and a bullet summary of what was delivered
   - Close the card: `fizzy card close <NUMBER>`

8. Update `WORKLOG.md`:
   - Find the card's row by its card reference
   - Change Status from `⬜` to `✅`
   - Set Notes to the merge commit short hash (7 chars)
   - Edit the row in-place with a targeted string replacement — do not reformat or reorder other rows
   - Never commit `WORKLOG.md` — it is gitignored
