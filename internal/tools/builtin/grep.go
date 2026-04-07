package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/chickenzord/zlaw/internal/llm"
)

// GrepFiles searches file contents for lines matching a regex pattern.
type GrepFiles struct{}

var grepSchema = []byte(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Regular expression pattern to search for."
    },
    "paths": {
      "type": "array",
      "items": {"type": "string"},
      "description": "List of file paths to search. At least one required."
    },
    "case_insensitive": {
      "type": "boolean",
      "description": "If true, matching is case-insensitive. Defaults to false."
    }
  },
  "required": ["pattern", "paths"]
}`)

type grepInput struct {
	Pattern         string   `json:"pattern"`
	Paths           []string `json:"paths"`
	CaseInsensitive bool     `json:"case_insensitive"`
}

func (GrepFiles) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "grep",
		Description: "Search file contents for lines matching a regular expression. Returns matching lines in the format \"path:linenum:line\".",
		InputSchema: grepSchema,
	}
}

func (GrepFiles) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input grepInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("grep: invalid input: %w", err)
	}
	if input.Pattern == "" {
		return "", fmt.Errorf("grep: pattern is required")
	}
	if len(input.Paths) == 0 {
		return "", fmt.Errorf("grep: at least one path is required")
	}

	rxStr := input.Pattern
	if input.CaseInsensitive {
		rxStr = "(?i)" + rxStr
	}
	rx, err := regexp.Compile(rxStr)
	if err != nil {
		return "", fmt.Errorf("grep: invalid pattern: %w", err)
	}

	var sb strings.Builder
	for _, path := range input.Paths {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(&sb, "%s: error: %v\n", path, err)
			continue
		}
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if rx.MatchString(line) {
				fmt.Fprintf(&sb, "%s:%d:%s\n", path, lineNum, line)
			}
		}
		f.Close()
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}
