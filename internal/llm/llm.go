// Package llm defines the LLM client interface and shared types.
package llm

import (
	"context"
	"errors"
)

// Role represents the speaker of a conversation turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolUse is a tool call requested by the model.
type ToolUse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input []byte `json:"input"` // raw JSON
}

// ToolResult is the response to a prior ToolUse.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// ContentBlock is one item in a message — text, a tool call, a tool result, or
// model thinking (extended reasoning). Exactly one field is non-zero.
type ContentBlock struct {
	Text       string      `json:"text,omitempty"`
	Thinking   string      `json:"thinking,omitempty"`
	ToolUse    *ToolUse    `json:"tool_use,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// Message is a single turn in the conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// TextContent returns all text blocks concatenated.
func (m Message) TextContent() string {
	var s string
	for _, b := range m.Content {
		s += b.Text
	}
	return s
}

// ThinkingContent returns all thinking blocks concatenated.
func (m Message) ThinkingContent() string {
	var s string
	for _, b := range m.Content {
		s += b.Thinking
	}
	return s
}

// ToolUses returns all tool-call blocks in this message.
func (m Message) ToolUses() []ToolUse {
	var out []ToolUse
	for _, b := range m.Content {
		if b.ToolUse != nil {
			out = append(out, *b.ToolUse)
		}
	}
	return out
}

// ToolDefinition describes a tool the model may call.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema []byte // JSON Schema object
}

// Request is the input to a single LLM call.
type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDefinition
	MaxTokens    int
}

// Response is the output of a single LLM call.
type Response struct {
	Message    Message
	StopReason string // "end_turn" | "tool_use" | "max_tokens"
	Usage      Usage
}

// Usage holds token counts for a single call.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ErrRateLimit is returned when the backend signals a rate-limit condition.
var ErrRateLimit = errors.New("llm: rate limited")

// Client is the interface every LLM backend must satisfy.
type Client interface {
	// Complete sends a request and returns the full response.
	Complete(ctx context.Context, req Request) (Response, error)
}
