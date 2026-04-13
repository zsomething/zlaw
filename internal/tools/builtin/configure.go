package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zsomething/zlaw/internal/llm"
)

// RuntimeLoader is the subset of config.Loader used by the Configure tool.
type RuntimeLoader interface {
	WriteRuntimeField(key, value string) error
	ReloadRuntime() error
}

// Configure is a built-in tool that lets the agent change runtime-configurable
// fields (e.g. the active LLM model) mid-loop without a process restart.
// Execution is synchronous: WriteRuntimeField → ReloadRuntime → result text.
type Configure struct {
	Loader RuntimeLoader
}

var configureSchema = []byte(`{
  "type": "object",
  "properties": {
    "field": {
      "type": "string",
      "enum": ["llm.model"],
      "description": "The runtime-configurable field to update."
    },
    "value": {
      "type": "string",
      "description": "The new value for the field."
    },
    "reason": {
      "type": "string",
      "description": "Brief explanation of why the change is being made."
    }
  },
  "required": ["field", "value", "reason"]
}`)

func (Configure) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name: "configure",
		Description: "Update a runtime-configurable agent setting (e.g. switch the active LLM model). " +
			"The change takes effect immediately — no restart required. " +
			"Only fields listed in the 'field' enum are configurable at runtime.",
		InputSchema: configureSchema,
	}
}

type configureInput struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

func (c Configure) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var in configureInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("configure: invalid input: %w", err)
	}
	if err := c.Loader.WriteRuntimeField(in.Field, in.Value); err != nil {
		return "", err
	}
	if err := c.Loader.ReloadRuntime(); err != nil {
		return "", fmt.Errorf("configure: reload failed after write: %w", err)
	}
	return fmt.Sprintf("Configuration updated: %s = %q", in.Field, in.Value), nil
}
