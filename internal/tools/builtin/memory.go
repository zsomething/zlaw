package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/llm"
)

// MemorySave upserts a long-term memory for the agent.
type MemorySave struct {
	Store agent.MemoryStore
}

var memorySaveSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "Stable identifier for this memory (e.g. 'user-pref-language'). Re-using the same ID overwrites the existing memory."
    },
    "content": {
      "type": "string",
      "description": "The text content to store."
    },
    "tags": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Optional list of tags for categorisation and retrieval."
    }
  },
  "required": ["id", "content"]
}`)

type memorySaveInput struct {
	ID      string   `json:"id"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

func (MemorySave) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "memory_save",
		Description: "Save or update a long-term memory. Use a stable ID so the same fact can be overwritten rather than duplicated. Returns the memory ID on success.",
		InputSchema: memorySaveSchema,
	}
}

func (t MemorySave) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input memorySaveInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("memory_save: invalid input: %w", err)
	}
	if input.ID == "" {
		return "", fmt.Errorf("memory_save: id is required")
	}
	if input.Content == "" {
		return "", fmt.Errorf("memory_save: content is required")
	}

	m := agent.Memory{
		ID:      input.ID,
		Tags:    input.Tags,
		Content: input.Content,
	}
	if err := t.Store.Save(m); err != nil {
		return "", fmt.Errorf("memory_save: %w", err)
	}
	return input.ID, nil
}

// MemoryRecall searches long-term memories by keyword and/or tag.
type MemoryRecall struct {
	Store agent.MemoryStore
}

var memoryRecallSchema = []byte(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Space-separated keywords to search for in memory content and tags. Leave empty to list all memories."
    }
  },
  "required": []
}`)

type memoryRecallInput struct {
	Query string `json:"query"`
}

func (MemoryRecall) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "memory_recall",
		Description: "Search long-term memories by keyword. Returns all matching memories with their IDs, tags, and content. Leave query empty to list all memories.",
		InputSchema: memoryRecallSchema,
	}
}

func (t MemoryRecall) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input memoryRecallInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("memory_recall: invalid input: %w", err)
	}

	var keywords []string
	if input.Query != "" {
		for _, w := range strings.Fields(input.Query) {
			keywords = append(keywords, w)
		}
	}

	memories, err := t.Store.Search(keywords)
	if err != nil {
		return "", fmt.Errorf("memory_recall: %w", err)
	}

	if len(memories) == 0 {
		return "(no memories found)", nil
	}

	var sb strings.Builder
	for i, m := range memories {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		fmt.Fprintf(&sb, "ID: %s\n", m.ID)
		if len(m.Tags) > 0 {
			fmt.Fprintf(&sb, "Tags: %s\n", strings.Join(m.Tags, ", "))
		}
		fmt.Fprintf(&sb, "Updated: %s\n", m.UpdatedAt.Format(time.RFC3339))
		fmt.Fprintf(&sb, "\n%s\n", m.Content)
	}
	return sb.String(), nil
}

// MemoryDelete removes a memory by ID.
type MemoryDelete struct {
	Store agent.MemoryStore
}

var memoryDeleteSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "The ID of the memory to delete."
    }
  },
  "required": ["id"]
}`)

type memoryDeleteInput struct {
	ID string `json:"id"`
}

func (MemoryDelete) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "memory_delete",
		Description: "Delete a long-term memory by its ID. No error is returned if the memory does not exist.",
		InputSchema: memoryDeleteSchema,
	}
}

func (t MemoryDelete) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input memoryDeleteInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("memory_delete: invalid input: %w", err)
	}
	if input.ID == "" {
		return "", fmt.Errorf("memory_delete: id is required")
	}

	if err := t.Store.Delete(input.ID); err != nil {
		return "", fmt.Errorf("memory_delete: %w", err)
	}
	return "deleted", nil
}
