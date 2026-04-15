// Package builtin provides built-in tools available to every agent.
package builtin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/zsomething/zlaw/internal/llm"
)

// CurrentTime returns the current UTC date and time.
type CurrentTime struct{}

var currentTimeSchema = []byte(`{
  "type": "object",
  "properties": {},
  "required": []
}`)

func (CurrentTime) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "time",
		Description: "Returns the current date and time in UTC (RFC 3339 format).",
		InputSchema: currentTimeSchema,
	}
}

func (CurrentTime) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return time.Now().UTC().Format(time.RFC3339), nil
}
