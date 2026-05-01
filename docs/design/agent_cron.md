# Agent: Cron Jobs

## Overview

Agents can schedule tasks to run automatically at specified times using cron expressions. Cron jobs are defined in `cron.toml` and executed by the agent's scheduler.

## Configuration

`cron.toml` in `$ZLAW_AGENT_HOME`:

```toml
[job.<id>]
schedule = "0 8 * * *"  # 5-field cron expression
task = "Check calendar and send daily summary"
disabled = false
```

## Cron Expression Format

Standard 5-field format:
```
 ┌───────────── minute (0-59)
 │ ┌───────────── hour (0-23)
 │ │ ┌───────────── day of month (1-31)
 │ │ │ ┌───────────── month (1-12)
 │ │ │ │ ┌───────────── day of week (0-6, Sunday=0)
 │ │ │ │
 * * * * *
```

Examples:
- `0 8 * * *` — 8:00 AM daily
- `30 14 * * 1-5` — 2:30 PM weekdays
- `0 */2 * * *` — every 2 hours

## Tools

| Tool | Description |
|------|-------------|
| `cronjob_list` | List all scheduled cron jobs |
| `cronjob_create` | Create a new cron job |
| `cronjob_delete` | Remove a cron job by ID |

## See Also

- [agent_standalone.md](./agent_standalone.md) — agent filesystem