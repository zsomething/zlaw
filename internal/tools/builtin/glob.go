package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsomething/zlaw/internal/llm"
)

// Glob finds files matching a glob pattern.
type Glob struct{}

var globSchema = []byte(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "Glob pattern to match files against (e.g. \"**/*.go\", \"src/**/*.ts\"). Supports ** for recursive matching."
    },
    "dir": {
      "type": "string",
      "description": "Directory to search in. Defaults to the current working directory."
    }
  },
  "required": ["pattern"]
}`)

type globInput struct {
	Pattern string `json:"pattern"`
	Dir     string `json:"dir"`
}

func (Glob) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "glob",
		Description: "Find files matching a glob pattern. Supports ** for recursive matching. Returns a newline-separated list of matching paths.",
		InputSchema: globSchema,
	}
}

func (Glob) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input globInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("glob: invalid input: %w", err)
	}
	if input.Pattern == "" {
		return "", fmt.Errorf("glob: pattern is required")
	}

	root := input.Dir
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("glob: %w", err)
		}
	}

	matches, err := globMatch(root, input.Pattern)
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}
	return strings.Join(matches, "\n"), nil
}

// globMatch returns file paths under root matching pattern.
// Supports ** for recursive directory matching.
func globMatch(root, pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(filepath.Join(root, pattern))
	}

	// Split at the first **/ segment.
	// e.g. "src/**/*.go" → prefix="src/", suffix="*.go"
	// e.g. "**/*.go"     → prefix="",     suffix="*.go"
	idx := strings.Index(pattern, "**/")
	prefix := pattern[:idx]
	suffix := pattern[idx+3:]

	searchRoot := filepath.Join(root, prefix)

	var matches []string
	err := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(searchRoot, path)
		if err != nil {
			return nil
		}
		// Match the relative path against the suffix pattern.
		// This handles cases like "internal/*.go" where the suffix itself has a dir component.
		ok, err := filepath.Match(suffix, rel)
		if err != nil {
			return fmt.Errorf("bad pattern %q: %w", suffix, err)
		}
		if !ok {
			// Also try matching just the filename for pure basename patterns like "*.go".
			ok, err = filepath.Match(suffix, filepath.Base(path))
			if err != nil {
				return fmt.Errorf("bad pattern %q: %w", suffix, err)
			}
		}
		if ok {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}
