// Package tools provides the tool registry and executor.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chickenzord/zlaw/internal/llm"
)

// Tool is the interface every tool must implement.
type Tool interface {
	// Definition returns the schema the LLM sees when deciding to call this tool.
	Definition() llm.ToolDefinition

	// Execute runs the tool with the given raw-JSON input and returns a result string.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// Registry holds a set of named tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool. Panics if name is already registered.
func (r *Registry) Register(t Tool) {
	name := t.Definition().Name
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tools: tool %q already registered", name))
	}
	r.tools[name] = t
}

// Definitions returns tool definitions for all registered tools, suitable for
// inclusion in an llm.Request.
func (r *Registry) Definitions() []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Definition())
	}
	return out
}

// Execute dispatches a single tool call and returns an llm.ToolResult.
func (r *Registry) Execute(ctx context.Context, call llm.ToolUse) llm.ToolResult {
	t, ok := r.tools[call.Name]
	if !ok {
		return llm.ToolResult{
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("unknown tool: %q", call.Name),
			IsError:   true,
		}
	}

	result, err := t.Execute(ctx, json.RawMessage(call.Input))
	if err != nil {
		return llm.ToolResult{
			ToolUseID: call.ID,
			Content:   err.Error(),
			IsError:   true,
		}
	}
	return llm.ToolResult{
		ToolUseID: call.ID,
		Content:   result,
	}
}

// ExecuteAll dispatches all tool calls in parallel and returns results in the
// same order.
func (r *Registry) ExecuteAll(ctx context.Context, calls []llm.ToolUse) []llm.ToolResult {
	results := make([]llm.ToolResult, len(calls))
	// Execute sequentially for now; parallel execution can be added later.
	for i, call := range calls {
		results[i] = r.Execute(ctx, call)
	}
	return results
}
