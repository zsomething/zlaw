package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/llm"
)

// WriteFile writes content to a file, creating parent directories as needed.
type WriteFile struct{}

var writeFileSchema = []byte(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute or relative path to the file to write."
    },
    "content": {
      "type": "string",
      "description": "Content to write to the file."
    }
  },
  "required": ["path", "content"]
}`)

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (WriteFile) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file and any missing parent directories. Overwrites the file if it already exists.",
		InputSchema: writeFileSchema,
	}
}

func (WriteFile) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input writeFileInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("write_file: invalid input: %w", err)
	}
	if input.Path == "" {
		return "", fmt.Errorf("write_file: path is required")
	}

	if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
		return "", fmt.Errorf("write_file: create parent dirs: %w", err)
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	return fmt.Sprintf("wrote %d bytes to %s", len(input.Content), input.Path), nil
}
