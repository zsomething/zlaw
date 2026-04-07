Commit staged or specified changes following this project's git workflow.

## Rules

### Commit message format
Use conventional commits: `<type>(<scope>): <description>`

- Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `ci`
- Scope: the package or component being changed (e.g. `config`, `llm`, `agent`, `tools`, `cli`)
- Description: brief imperative phrase explaining *what changed and why*, not just what file was touched
- No body or footer unless the change genuinely needs explanation beyond the subject line
- Keep the subject line under 72 characters

Examples:
```
feat(config): load SOUL.md and IDENTITY.md at agent startup
fix(llm): retry on 429 before surfacing error to caller
refactor(agent): extract context builder into its own type
chore: add .fizzy.yaml to gitignore
```

### What to stage and commit
- Only stage files directly relevant to the feature or fix being committed
- If changes span multiple scopes, split into separate commits (one per scope)
- Do not bundle unrelated changes in a single commit

### Language restrictions
- Never mention Claude, AI models, LLMs, issue trackers, or external tools in commit messages or PR descriptions
- Describe the code change itself — not the process used to produce it

### .gitignore hygiene
- Before committing, check if any generated, local, or tool-specific files should be ignored
- Add them to `.gitignore` in the same commit where they first appear, using the `chore` type if nothing else fits

## Workflow

1. Review `git diff` and `git status` to understand what changed
2. Check for files that should be added to `.gitignore` before staging
3. Group changes by scope; plan separate commits if needed
4. Stage only the relevant files for the current commit
5. Write the commit message following the format above
6. Commit. If a pre-commit hook fails, fix the issue and create a new commit — never use `--no-verify`
