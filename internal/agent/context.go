package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chickenzord/zlaw/internal/config"
)

// BuildPrefill expands a list of context sources into a preamble string that
// can be prepended to the first user message of a new session. Returns an
// empty string when sources is empty.
//
// Supported sources:
//   - "cwd"              — current working directory
//   - "datetime"         — current date and time (RFC3339)
//   - "file:<path>"      — contents of a file relative to agentDir
func BuildPrefill(agentDir string, sources []string) (string, error) {
	if len(sources) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("[Session context]\n")
	for _, src := range sources {
		switch {
		case src == "cwd":
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("prefill cwd: %w", err)
			}
			fmt.Fprintf(&b, "cwd: %s\n", cwd)
		case src == "datetime":
			fmt.Fprintf(&b, "datetime: %s\n", time.Now().Format(time.RFC3339))
		case strings.HasPrefix(src, "file:"):
			rel := strings.TrimPrefix(src, "file:")
			path := filepath.Join(agentDir, rel)
			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("prefill file %q: %w", path, err)
			}
			fmt.Fprintf(&b, "file:%s\n%s\n", rel, strings.TrimSpace(string(data)))
		default:
			return "", fmt.Errorf("prefill: unknown source %q (supported: cwd, datetime, file:<path>)", src)
		}
	}
	return b.String(), nil
}

// BuildSystemPrompt assembles the system prompt from the agent's personality
// files. SOUL.md provides the base character; IDENTITY.md provides situational
// context. Either may be empty.
func BuildSystemPrompt(p config.Personality) string {
	var b strings.Builder
	if p.Soul != "" {
		b.WriteString(strings.TrimSpace(p.Soul))
	}
	if p.Identity != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(p.Identity))
	}
	return b.String()
}
