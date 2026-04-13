package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/skills"
)

// SkillLoad loads the full body of a named skill so the agent can use it.
type SkillLoad struct {
	// Skills is the indexed map of available skills keyed by name.
	Skills map[string]skills.Skill
}

var skillLoadSchema = []byte(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the skill to load (as listed in [Available Skills])."
    }
  },
  "required": ["name"]
}`)

type skillLoadInput struct {
	Name string `json:"name"`
}

func (SkillLoad) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "skill_load",
		Description: "Load the full instructions for a named skill. Call this when you decide a skill is relevant to the current task. Returns the skill body as a string.",
		InputSchema: skillLoadSchema,
	}
}

func (t SkillLoad) Execute(_ context.Context, raw json.RawMessage) (string, error) {
	var input skillLoadInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("skill_load: invalid input: %w", err)
	}
	if input.Name == "" {
		return "", fmt.Errorf("skill_load: name is required")
	}

	s, ok := t.Skills[input.Name]
	if !ok {
		return "", fmt.Errorf("skill_load: skill %q not found", input.Name)
	}
	return s.Body, nil
}
