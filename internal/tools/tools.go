// Package tools provides the tool registry and executor.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/zsomething/zlaw/internal/llm"
)

// Tool is the interface every tool must implement.
type Tool interface {
	// Definition returns the schema the LLM sees when deciding to call this tool.
	Definition() llm.ToolDefinition

	// Execute runs the tool with the given raw-JSON input and returns a result string.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

const defaultMaxResultBytes = 10000

// Registry holds a set of named tools.
type Registry struct {
	tools          map[string]Tool
	allowed        map[string]struct{} // nil means all tools are allowed
	maxResultBytes int                 // 0 means use default; negative means disabled
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// SetMaxResultBytes configures the maximum byte length of a tool result before
// it is truncated. Zero uses the default (10 000 bytes). Negative disables
// truncation entirely.
func (r *Registry) SetMaxResultBytes(n int) {
	r.maxResultBytes = n
}

// truncate applies the configured result-size limit to s. If the result
// exceeds the limit a notice is appended so the model knows output was cut.
func (r *Registry) truncate(s string) string {
	limit := r.maxResultBytes
	if limit == 0 {
		limit = defaultMaxResultBytes
	}
	if limit < 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + fmt.Sprintf("\n[truncated: original %d chars]", len(s))
}

// SetAllowlist restricts which tools are visible and executable to the named
// set. An empty slice clears any previous allowlist (all tools allowed).
func (r *Registry) SetAllowlist(names []string) {
	if len(names) == 0 {
		r.allowed = nil
		return
	}
	r.allowed = make(map[string]struct{}, len(names))
	for _, n := range names {
		r.allowed[n] = struct{}{}
	}
}

// isAllowed reports whether a tool name is permitted by the allowlist.
// If no allowlist is set, all tools are permitted.
func (r *Registry) isAllowed(name string) bool {
	if r.allowed == nil {
		return true
	}
	_, ok := r.allowed[name]
	return ok
}

// Get returns a registered tool by name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// Register adds a tool. Panics if name is already registered.
func (r *Registry) Register(t Tool) {
	name := t.Definition().Name
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tools: tool %q already registered", name))
	}
	r.tools[name] = t
}

// Definitions returns tool definitions for all registered tools that are
// permitted by the allowlist, suitable for inclusion in an llm.Request.
func (r *Registry) Definitions() []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		if r.isAllowed(t.Definition().Name) {
			out = append(out, t.Definition())
		}
	}
	return out
}

// Execute dispatches a single tool call and returns an llm.ToolResult.
func (r *Registry) Execute(ctx context.Context, call llm.ToolUse) llm.ToolResult {
	if !r.isAllowed(call.Name) {
		return llm.ToolResult{
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("tool %q is not allowed", call.Name),
			IsError:   true,
		}
	}
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
		Content:   r.truncate(result),
	}
}

// ExecuteAll dispatches all tool calls concurrently and returns results in the
// same order as calls.
func (r *Registry) ExecuteAll(ctx context.Context, calls []llm.ToolUse) []llm.ToolResult {
	results := make([]llm.ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(i int, call llm.ToolUse) {
			defer wg.Done()
			results[i] = r.Execute(ctx, call)
		}(i, call)
	}
	wg.Wait()
	return results
}
