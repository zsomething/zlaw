---
name: fizzy-workflow
description: High-level workflows for managing work using Fizzy cards — start, work on, complete, and delegate cards using the Fizzy CLI.
compatibility: Requires fizzy in PATH. Install fizzy from https://github.com/basecamp/fizzy-cli/releases (single-file binary).
metadata:
  {
    "author": "akhy",
    "version": "1.1.0",
    "openclaw":
      {
        "emoji": "🃏",
        "homepage": "https://github.com/basecamp/fizzy-cli",
        "requires": { "bins": ["fizzy"] },
        "install":
          [
            {
              "id": "download-fizzy",
              "kind": "download",
              "url": "https://github.com/basecamp/fizzy-cli/releases",
              "bins": ["fizzy"],
              "label": "Download Fizzy CLI",
            },
          ],
      },
  }
---

# Fizzy Workflow Skill

High-level workflows for managing work using Fizzy cards. This skill builds on top of the base `fizzy` skill to provide structured workflows for starting, working on, and completing cards.

## Prerequisites

- **`fizzy`** — Fizzy CLI binary. Download from [github.com/basecamp/fizzy-cli/releases](https://github.com/basecamp/fizzy-cli/releases) and place in your PATH.
- **`jq`** (optional) — Required as fallback if fizzy's built-in `--jq` flag is unavailable (currently unreleased). Install via `brew install jq` or `apt install jq`.

This skill also depends on the **base `fizzy` skill** for raw Fizzy CLI operations. If not already installed, install it from [github.com/basecamp/fizzy-cli/tree/master/skills/fizzy](https://github.com/basecamp/fizzy-cli/tree/master/skills/fizzy).

> **Note on `--jq` flag:** Examples in this skill use fizzy's built-in `--jq` filter flag, which is currently unreleased. Helper scripts fall back to external `jq` automatically. The fizzy CLI outputs JSON by default — no output format flag is needed. For inline patterns, replace `fizzy ... --jq 'EXPR'` with `fizzy ... | jq 'EXPR'` if needed.

---

## Quick Reference

| Workflow | When to Use |
|----------|-------------|
| Start Card | Begin work on a card, move to Doing, assign to self |
| Complete Card | Finish card, post commit link, close |
| Update Progress | Mark step complete, post progress |
| Assign to Human | Delegate to human for manual tasks |

---

## Workflows

### 0. Planning Cards (optional)

**When:** Breaking down a larger task into multiple cards before starting work

**Steps:**
1. Write a JSON plan file describing all cards and their steps
2. Validate with `--dry-run` before creating
3. Run the plan to create all cards in one invocation
4. Then pick a card and follow the Start Card workflow

**Plan file format:**
```json
{
  "board_id": "<board_id>",
  "cards": [
    {
      "title": "Card title",
      "description": "Optional description (markdown or HTML)",
      "tags": ["backend", "urgent"],
      "steps": [
        "Step one",
        "Step two"
      ]
    }
  ]
}
```

`description`, `tags`, and `steps` are optional per card. Before assigning tags, run `fizzy tag list` to see existing ones — **prefer reusing existing tags** over creating new ones to avoid duplicates.

If a card depends on another card, note it clearly in the description, e.g.:
```
Depends on: #42 (Set up authentication)
```

**Example:**
```bash
# Dry-run first to verify
python3 fizzy-workflow/scripts/fizzy-plan-create.py plan.json --dry-run

# Create all cards
python3 fizzy-workflow/scripts/fizzy-plan-create.py plan.json
```

**Output (agent-friendly JSON):**
```json
{
  "ok": true,
  "dry_run": false,
  "created": [
    {"number": 42, "title": "Card title", "tags": ["backend"], "steps_created": 2}
  ]
}
```

On failure (e.g. mid-way through), `partial` is included so the agent knows what was already created:
```json
{
  "ok": false,
  "error": "step create failed: ...",
  "partial": {"number": 42, "title": "Card title", "steps_created": 1}
}
```

---

### 1. Starting a Card

**When:** Beginning work on a new card

**Steps:**
1. Read the card thoroughly:
   - `fizzy card show CARD_NUMBER` — title, description, tags, steps
   - `fizzy comment list --card CARD_NUMBER` — full comment history
   - Understand all requirements, context, and prior discussion before doing anything else
2. Check current git branch:
   - If on `main`/`master`: checkout a new feature branch named after the feature (e.g. `feat/short-description`) — **never include card/fizzy numbers in branch names**
   - If on an existing feature branch: confirm with the user whether this branch is relevant to the card before continuing
   - It's acceptable to work on multiple cards in the same branch, but each card **must be committed separately**
3. Find "Doing" column ID: `fizzy column list --board BOARD_ID`
4. Move to "Doing" column: `fizzy card column CARD_NUMBER --column <doing_column_id>`
5. Assign to self: `fizzy card self-assign CARD_NUMBER`

**Example:**
```bash
# Read card thoroughly
fizzy card show 15
fizzy comment list --card 15

# Check branch and prepare
git branch --show-current
# If on main/master:
git checkout -b feat/short-description
# If on a feature branch that may not be relevant: STOP and confirm with user

# Get column IDs
fizzy column list --board BOARD_ID --jq '[.data[] | {id, name}]'

# Move to Doing
fizzy card column 15 --column <doing_column_id>

# Assign to self
fizzy card self-assign 15
```

**What to tell the user:**
```
Card #X moved to "Doing" and assigned. Starting implementation...
```

---

### 2. Working on a Card

**During implementation:**

**Mark steps complete:**
```bash
fizzy step update STEP_ID --card CARD_NUMBER --completed
```

**Post SHORT progress comments:**
- Only for completed steps or important milestones
- Keep concise (1-2 sentences) to save tokens
- Include brief justification for design/decision changes

**Example progress comment:**
```bash
fizzy comment create --card 12 --body "$(cat <<'EOF'
<p>✅ Feature X completed</p>
<p>Migration applied: <code>20260211040415_init</code></p>
EOF
)"
```

**IMPORTANT:**
- Comments are visible to team, so be clear but brief
- Focus on WHAT was done, not HOW
- Skip obvious/trivial updates

---

### 3. Completing a Card

**When:** All work is done and committed

**Required steps:**

#### Step 1: Commit to Git
Use conventional commit format, **NEVER mention Fizzy** (it's external to the project):

```bash
git add <files>
git commit -m "$(cat <<'EOF'
feat(scope): implement feature X

Detailed description of what was implemented.

Key changes:
- Feature A
- Feature B
- Feature C
EOF
)"
```

**Conventional commit types:**
- `feat`: New features
- `fix`: Bug fixes
- `docs`: Documentation
- `refactor`: Code refactoring
- `test`: Adding tests
- `chore`: Maintenance tasks

#### Step 2: Land changes into main branch

**The card must only be closed after changes are in `main`/`master`.** Infer the right approach from context, or ask the user if unclear:

**Option A — Create PR/MR for review** (preferred for team projects or when changes need review):
```bash
git push origin <branch>
# Then create a PR/MR via gh, glab, or the platform's CLI
gh pr create --title "feat: short description" --body "..."
```
Wait for the PR/MR to be merged before proceeding to close the card.

**Option B — Merge directly to main** (for solo projects or pre-approved changes):
```bash
git checkout main
git merge --no-ff <branch>   # NEVER fast-forward
git push origin main
```

#### Step 3: Get Commit Hash and Repo URL
```bash
COMMIT_HASH=$(git log -1 --format="%H")
REPO_URL=$(git remote get-url origin | sed 's/git@github.com:/https:\/\/github.com\//' | sed 's/\.git$//')
```

#### Step 4: Post Completion Comment
```bash
fizzy comment create --card NUMBER --body "$(cat <<'EOF'
<p>✅ Completed and merged to main</p>
<p><br></p>
<p>Commit: <a href="REPO_URL/commit/COMMIT_HASH">SHORT_HASH</a></p>
<p><br></p>
<p><strong>Summary:</strong></p>
<ul>
<li>Key deliverable 1</li>
<li>Key deliverable 2</li>
<li>Key deliverable 3</li>
</ul>
EOF
)"
```

#### Step 5: Close Card
```bash
fizzy card close CARD_NUMBER
```

**What to tell the user:**
```
✅ Card #X Complete!

**Commit:** [hash](github_url)

**Delivered:**
- Feature A
- Feature B
- Feature C

**Card Status:** ✅ Closed
```

---

### 4. Assigning to Human

**When to assign to human:**
- Tasks that can only be done manually by human
- Tasks requiring confirmation/approval
- Infrastructure decisions (deployment, hosting, CI/CD platforms)
- Security reviews or sensitive configurations
- UI/UX design decisions

**How to find human user ID:**
```bash
# List all users to find the right person
fizzy user list --jq '[.data[] | {id, name, email: .email_address}]'
```

**Assign:**
```bash
fizzy card assign CARD_NUMBER --user <human_user_id>
```

**Post a comment explaining why:**
```bash
fizzy comment create --card NUMBER --body "$(cat <<'EOF'
<p>🤝 Assigned for human decision</p>
<p><br></p>
<p><strong>Reason:</strong> [brief explanation]</p>
<p><strong>Questions:</strong></p>
<ul>
<li>Question 1?</li>
<li>Question 2?</li>
</ul>
EOF
)"
```

---

## Best Practices

### Comment Guidelines
✅ **DO:**
- Post when steps complete
- Post for design/decision changes
- Post for important milestones
- Keep to 1-2 sentences
- Use clear, actionable language

❌ **DON'T:**
- Post for every minor action
- Write long explanations
- Duplicate information already in code
- Use internal/technical jargon unnecessarily

### Git Commit Guidelines
✅ **DO:**
- Use conventional commit format
- Focus on WHAT and WHY
- Include relevant scope
- List key changes in body

❌ **DON'T:**
- Mention Fizzy (it's external to project)
- Reference card numbers in commits
- Write vague messages
- Skip the commit body for complex changes

### Card Management
✅ **DO:**
- Move cards through workflow stages
- Assign appropriately (self vs human)
- Close cards when truly complete
- Update steps as you progress
- Add relevant tags to cards for discoverability
- Run `fizzy tag list` and reuse existing tags before creating new ones
- Note dependencies on other cards in the description (`Depends on: #N`)

❌ **DON'T:**
- Leave cards in wrong columns
- Close cards with incomplete work
- Forget to assign cards
- Skip step updates
- Create duplicate or near-duplicate tags
- Start a card before its dependencies are complete

---

## Common Patterns

### Pattern: Multi-Step Card Workflow
```bash
# 0. Prepare branch (from main/master)
git checkout -b feat/short-description

# 1. Get board and column info
BOARD_ID=$(fizzy board list --jq '.data[0].id')
DOING_COL=$(fizzy column list --board $BOARD_ID --jq '.data[] | select(.name == "Doing") | .id')

# 2. Start the card
fizzy card column 15 --column $DOING_COL
fizzy card self-assign 15

# 3. Work and mark steps
fizzy step update STEP_1_ID --card 15 --completed
fizzy step update STEP_2_ID --card 15 --completed
# ... continue implementation ...

# 4. Commit
git add <files>
git commit -m "feat: implement feature"

# 5. Complete and close
COMMIT_HASH=$(git log -1 --format="%H")
SHORT_HASH=$(git log -1 --format="%h")
REPO_URL=$(git remote get-url origin | sed 's/git@github.com:/https:\/\/github.com\//' | sed 's/\.git$//')

fizzy comment create --card 15 --body "<p>✅ Done. Commit: <a href='$REPO_URL/commit/$COMMIT_HASH'>$SHORT_HASH</a></p>"
fizzy card close 15
```

### Pattern: No-Steps Card Workflow
```bash
# 1. Start
fizzy card column 20 --column <doing_col_id>
fizzy card self-assign 20

# 2. Work (no steps to update)
# ... implementation ...

# 3. Commit and complete
git commit -m "feat: implement X"
fizzy comment create --card 20 --body "<p>✅ Done</p>"
fizzy card close 20
```

### Pattern: Delegate to Human
```bash
# Find human user
HUMAN_USER=$(fizzy user list --jq '.data[] | select(.role == "owner") | .id')

# Assign and explain
fizzy card assign 26 --user $HUMAN_USER
fizzy comment create --card 26 --body "$(cat <<'EOF'
<p>🤝 Needs human decision on deployment platform</p>
<p>Options: AWS, GCP, or DigitalOcean?</p>
EOF
)"
```

---

## Integration with Base Fizzy Skill

This workflow skill **uses** the base `fizzy` skill for all Fizzy CLI operations. When you need to:
- Search cards: Use base `fizzy` skill
- List boards/columns: Use base `fizzy` skill
- View card details: Use base `fizzy` skill
- Execute workflow: Use **this** skill

**Example decision tree:**
- User asks "Which cards are available?" → Use base `fizzy` skill
- User says "Work on card #15" → Use **this workflow** skill
- User asks "Show card #15 details" → Use base `fizzy` skill
- User says "Complete card #15" → Use **this workflow** skill

---

## Token Optimization

### Keep Comments Concise
Instead of:
```html
<p>I have successfully completed the implementation of the feature which includes all components as specified in the requirements document. The changes have been tested and committed to the repository successfully.</p>
```

Use:
```html
<p>✅ Feature completed and tested. Changes committed.</p>
```

### Batch Operations
When possible, complete multiple steps before posting a single summary comment:
```bash
# Instead of commenting after each step
fizzy step update STEP1 --card 12 --completed
# [work]
fizzy step update STEP2 --card 12 --completed
# [work]
fizzy step update STEP3 --card 12 --completed

# Post one summary
fizzy comment create --card 12 --body "<p>✅ Steps 1-3 complete</p>"
```

---

## Helper Scripts

### `scripts/fizzy-plan-create.py <plan.json> [--dry-run]`
Create multiple cards with steps from a JSON plan file in a single invocation. Use `--dry-run` to validate the plan before executing. Outputs JSON with created card numbers.

```bash
python3 fizzy-workflow/scripts/fizzy-plan-create.py plan.json --dry-run
python3 fizzy-workflow/scripts/fizzy-plan-create.py plan.json
```

### `scripts/fizzy-context.sh <board_id>`
Get column IDs and git remote for a board — run this before starting a card to look up the "Doing" column ID.

```bash
bash fizzy-workflow/scripts/fizzy-context.sh <board_id>
```

### `scripts/fizzy-open-cards.sh <board_id>`
List all open cards on a board with number, title, and current assignees — useful for picking what to work on next.

```bash
bash fizzy-workflow/scripts/fizzy-open-cards.sh <board_id>
```
