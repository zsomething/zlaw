package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/ctxkey"
	"github.com/zsomething/zlaw/internal/llm"
)

// CronReader is the interface used by read-only cron tools.
type CronReader interface {
	AgentDir() string
}

// CronWriter is the interface used by cron tools that mutate cron.toml.
// It extends CronReader with a reload trigger so the scheduler picks up
// changes immediately without waiting for the file watcher.
type CronWriter interface {
	CronReader
	ReloadCron()
}

// --- list_cronjobs ---

// ListCronjobs lists all cron jobs defined in cron.toml.
type ListCronjobs struct {
	Reader CronReader
}

func (ListCronjobs) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "list_cronjobs",
		Description: "List all scheduled cron jobs configured for this agent.",
		InputSchema: []byte(`{"type":"object","properties":{}}`),
	}
}

func (t ListCronjobs) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	cfg, err := config.LoadCronConfig(t.Reader.AgentDir())
	if err != nil {
		return "", fmt.Errorf("list_cronjobs: %w", err)
	}
	if len(cfg.Jobs) == 0 {
		return "No cron jobs configured.", nil
	}
	var sb strings.Builder
	for _, j := range cfg.Jobs {
		fmt.Fprintf(&sb, "id=%q schedule=%q task=%q target=%q\n",
			j.ID, j.Schedule, j.Task, j.Target)
	}
	return sb.String(), nil
}

// --- create_cronjob ---

// CreateCronjob adds a new cron job to cron.toml.
type CreateCronjob struct {
	Writer CronWriter
}

var createCronjobSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "Unique identifier for the cron job. Must not conflict with existing IDs."
    },
    "schedule": {
      "type": "string",
      "description": "Standard 5-field cron expression, e.g. \"0 8 * * *\" for 8 AM daily."
    },
    "task": {
      "type": "string",
      "description": "The prompt sent to the agent when the job fires."
    },
    "target": {
      "type": "string",
      "description": "Push target address, e.g. \"telegram:123456789\". Defaults to the current session's channel when omitted."
    }
  },
  "required": ["id", "schedule", "task"]
}`)

func (CreateCronjob) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name: "create_cronjob",
		Description: "Create a new scheduled cron job. The job will run the given task prompt " +
			"on the configured schedule and deliver the result to the target channel. " +
			"Omit target to default to the current session's channel.",
		InputSchema: createCronjobSchema,
	}
}

type createCronjobInput struct {
	ID       string `json:"id"`
	Schedule string `json:"schedule"`
	Task     string `json:"task"`
	Target   string `json:"target"`
}

func (t CreateCronjob) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var in createCronjobInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("create_cronjob: invalid input: %w", err)
	}
	if in.ID == "" || in.Schedule == "" || in.Task == "" {
		return "", fmt.Errorf("create_cronjob: id, schedule, and task are required")
	}

	// Default target to the current session's source channel.
	if in.Target == "" {
		if ch, ok := ctx.Value(ctxkey.SourceChannel).(string); ok && ch != "" {
			in.Target = ch
		}
	}

	cfg, err := config.LoadCronConfig(t.Writer.AgentDir())
	if err != nil {
		return "", fmt.Errorf("create_cronjob: load cron config: %w", err)
	}

	// Reject duplicate IDs.
	for _, j := range cfg.Jobs {
		if j.ID == in.ID {
			return "", fmt.Errorf("create_cronjob: job with id %q already exists", in.ID)
		}
	}

	cfg.Jobs = append(cfg.Jobs, config.CronJobConfig{
		ID:       in.ID,
		Schedule: in.Schedule,
		Task:     in.Task,
		Target:   in.Target,
	})

	if err := config.WriteCronConfig(t.Writer.AgentDir(), cfg); err != nil {
		return "", fmt.Errorf("create_cronjob: write: %w", err)
	}
	t.Writer.ReloadCron()

	return fmt.Sprintf("Cron job %q created (schedule: %s, target: %s).", in.ID, in.Schedule, in.Target), nil
}

// --- delete_cronjob ---

// DeleteCronjob removes a cron job from cron.toml by ID.
type DeleteCronjob struct {
	Writer CronWriter
}

var deleteCronjobSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "ID of the cron job to delete."
    }
  },
  "required": ["id"]
}`)

func (DeleteCronjob) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "delete_cronjob",
		Description: "Delete a scheduled cron job by its ID.",
		InputSchema: deleteCronjobSchema,
	}
}

type deleteCronjobInput struct {
	ID string `json:"id"`
}

func (t DeleteCronjob) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var in deleteCronjobInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "", fmt.Errorf("delete_cronjob: invalid input: %w", err)
	}
	if in.ID == "" {
		return "", fmt.Errorf("delete_cronjob: id is required")
	}

	cfg, err := config.LoadCronConfig(t.Writer.AgentDir())
	if err != nil {
		return "", fmt.Errorf("delete_cronjob: load cron config: %w", err)
	}

	filtered := cfg.Jobs[:0]
	found := false
	for _, j := range cfg.Jobs {
		if j.ID == in.ID {
			found = true
			continue
		}
		filtered = append(filtered, j)
	}
	if !found {
		return "", fmt.Errorf("delete_cronjob: no job with id %q", in.ID)
	}
	cfg.Jobs = filtered

	if err := config.WriteCronConfig(t.Writer.AgentDir(), cfg); err != nil {
		return "", fmt.Errorf("delete_cronjob: write: %w", err)
	}
	t.Writer.ReloadCron()

	return fmt.Sprintf("Cron job %q deleted.", in.ID), nil
}
