package llm

import (
	"context"
	"fmt"
)

// MockClient is a deterministic LLM client for unit tests.
// Each call pops the next response from Responses; if exhausted it returns an error.
type MockClient struct {
	Responses []Response
	Requests  []Request // recorded for assertion
	idx       int
}

// Complete returns the next canned response.
func (m *MockClient) Complete(_ context.Context, req Request) (Response, error) {
	m.Requests = append(m.Requests, req)
	if m.idx >= len(m.Responses) {
		return Response{}, fmt.Errorf("mock: no more responses (call %d)", m.idx+1)
	}
	resp := m.Responses[m.idx]
	m.idx++
	return resp, nil
}

// Reset clears recorded requests and resets the response pointer.
func (m *MockClient) Reset() {
	m.Requests = nil
	m.idx = 0
}

// TextResponse is a convenience constructor for a simple text response.
func TextResponse(text string) Response {
	return Response{
		Message: Message{
			Role:    RoleAssistant,
			Content: []ContentBlock{{Text: text}},
		},
		StopReason: "end_turn",
	}
}

// ToolUseResponse is a convenience constructor for a tool-call response.
func ToolUseResponse(id, name string, input []byte) Response {
	return Response{
		Message: Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{ToolUse: &ToolUse{ID: id, Name: name, Input: input}},
			},
		},
		StopReason: "tool_use",
	}
}
