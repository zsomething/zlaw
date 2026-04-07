package agent

import (
	"strings"

	"github.com/chickenzord/zlaw/internal/config"
)

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
