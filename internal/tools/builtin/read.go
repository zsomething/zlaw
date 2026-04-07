package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/chickenzord/zlaw/internal/llm"
)

// ReadFile reads a file's contents with optional line offset and limit.
type ReadFile struct{}

var readFileSchema = []byte(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute or relative path to the file to read."
    },
    "offset": {
      "type": "integer",
      "description": "Zero-based line number to start reading from. Defaults to 0.",
      "minimum": 0
    },
    "limit": {
      "type": "integer",
      "description": "Maximum number of lines to return. Omit or set to 0 for all lines.",
      "minimum": 0
    }
  },
  "required": ["path"]
}`)

type readFileInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (ReadFile) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file contents as text. Use offset and limit to read a specific range of lines.",
		InputSchema: readFileSchema,
	}
}

func (ReadFile) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input readFileInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("read_file: invalid input: %w", err)
	}
	if input.Path == "" {
		return "", fmt.Errorf("read_file: path is required")
	}

	data, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Apply offset.
	if input.Offset > 0 {
		if input.Offset >= len(lines) {
			return "", nil
		}
		lines = lines[input.Offset:]
	}

	// Apply limit.
	if input.Limit > 0 && input.Limit < len(lines) {
		lines = lines[:input.Limit]
	}

	return strings.Join(lines, "\n"), nil
}
