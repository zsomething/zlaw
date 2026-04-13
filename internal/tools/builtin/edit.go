package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/zsomething/zlaw/internal/llm"
)

// EditFile performs a targeted string replacement in a file.
// It fails if the old string is not found or appears more than once.
type EditFile struct{}

var editFileSchema = []byte(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute or relative path to the file to edit."
    },
    "old_string": {
      "type": "string",
      "description": "The exact string to replace. Must appear exactly once in the file."
    },
    "new_string": {
      "type": "string",
      "description": "The string to replace old_string with."
    }
  },
  "required": ["path", "old_string", "new_string"]
}`)

type editFileInput struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (EditFile) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "edit_file",
		Description: "Replace an exact string in a file. Fails if the string is not found or appears more than once (ambiguous match). Use read_file first to confirm the exact text.",
		InputSchema: editFileSchema,
	}
}

func (EditFile) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input editFileInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("edit_file: invalid input: %w", err)
	}
	if input.Path == "" {
		return "", fmt.Errorf("edit_file: path is required")
	}
	if input.OldString == "" {
		return "", fmt.Errorf("edit_file: old_string is required")
	}

	data, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	content := string(data)

	count := strings.Count(content, input.OldString)
	switch count {
	case 0:
		return "", fmt.Errorf("edit_file: old_string not found in %s", input.Path)
	case 1:
		// exactly one match — proceed
	default:
		return "", fmt.Errorf("edit_file: old_string found %d times in %s (ambiguous match)", count, input.Path)
	}

	updated := strings.Replace(content, input.OldString, input.NewString, 1)
	if err := os.WriteFile(input.Path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}

	return fmt.Sprintf("replaced 1 occurrence in %s", input.Path), nil
}
