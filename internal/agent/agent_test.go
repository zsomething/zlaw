package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
)

// --- helpers ---

func newAgent(client llm.Client, tools agent.ToolExecutor) *agent.Agent {
	return agent.New("test-agent", client, tools, agent.NewHistory(), discardLogger())
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// noopTools satisfies ToolExecutor with no registered tools.
type noopTools struct{}

func (noopTools) Definitions() []llm.ToolDefinition { return nil }
func (noopTools) ExecuteAll(_ context.Context, calls []llm.ToolUse) []llm.ToolResult {
	return nil
}

// stubTools returns fixed results for all tool calls.
type stubTools struct {
	defs    []llm.ToolDefinition
	results map[string]string // tool name → result content
}

func (s stubTools) Definitions() []llm.ToolDefinition { return s.defs }
func (s stubTools) ExecuteAll(_ context.Context, calls []llm.ToolUse) []llm.ToolResult {
	out := make([]llm.ToolResult, len(calls))
	for i, c := range calls {
		content, ok := s.results[c.Name]
		if !ok {
			content = "unknown tool"
		}
		out[i] = llm.ToolResult{ToolUseID: c.ID, Content: content}
	}
	return out
}

// --- tests ---

func TestAgent_PlainTextResponse(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{llm.TextResponse("Hello!")},
	}
	a := newAgent(mock, noopTools{})

	got, err := a.Run(context.Background(), "s1", "hi", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != "Hello!" {
		t.Fatalf("expected %q, got %q", "Hello!", got.Text)
	}
	if len(mock.Requests) != 1 {
		t.Fatalf("expected 1 llm call, got %d", len(mock.Requests))
	}
}

func TestAgent_ToolCallThenResponse(t *testing.T) {
	toolInput, _ := json.Marshal(map[string]any{})
	mock := &llm.MockClient{
		Responses: []llm.Response{
			llm.ToolUseResponse("call-1", "current_time", toolInput),
			llm.TextResponse("It is now 2026-04-07T00:00:00Z"),
		},
	}
	tools := stubTools{
		defs:    []llm.ToolDefinition{{Name: "current_time", Description: "time"}},
		results: map[string]string{"current_time": "2026-04-07T00:00:00Z"},
	}
	a := newAgent(mock, tools)

	got, err := a.Run(context.Background(), "s1", "what time is it?", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != "It is now 2026-04-07T00:00:00Z" {
		t.Fatalf("unexpected response: %q", got.Text)
	}
	if len(mock.Requests) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(mock.Requests))
	}
}

func TestAgent_HistoryAccumulates(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{
			llm.TextResponse("First"),
			llm.TextResponse("Second"),
		},
	}
	a := newAgent(mock, noopTools{})

	_, _ = a.Run(context.Background(), "s1", "turn 1", "")
	_, _ = a.Run(context.Background(), "s1", "turn 2", "")

	// Second request should include 3 messages: user1, assistant1, user2.
	if len(mock.Requests) != 2 {
		t.Fatalf("expected 2 llm calls, got %d", len(mock.Requests))
	}
	secondReq := mock.Requests[1]
	if len(secondReq.Messages) != 3 {
		t.Fatalf("expected 3 messages in second request, got %d", len(secondReq.Messages))
	}
}

func TestAgent_SessionsAreIsolated(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{
			llm.TextResponse("s1 response"),
			llm.TextResponse("s2 response"),
		},
	}
	a := newAgent(mock, noopTools{})

	_, _ = a.Run(context.Background(), "s1", "hello from s1", "")
	_, _ = a.Run(context.Background(), "s2", "hello from s2", "")

	// s2 request must contain only 1 message (its own input), not s1's history.
	s2Req := mock.Requests[1]
	if len(s2Req.Messages) != 1 {
		t.Fatalf("s2 request should have 1 message, got %d", len(s2Req.Messages))
	}
}

func TestAgent_MaxTokensError(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{{
			Message:    llm.Message{Role: llm.RoleAssistant},
			StopReason: "max_tokens",
		}},
	}
	a := newAgent(mock, noopTools{})

	_, err := a.Run(context.Background(), "s1", "tell me everything", "")
	if err == nil {
		t.Fatal("expected error for max_tokens stop reason")
	}
}

func TestAgent_LLMError(t *testing.T) {
	mock := &llm.MockClient{} // no responses → returns error
	a := newAgent(mock, noopTools{})

	_, err := a.Run(context.Background(), "s1", "hello", "")
	if err == nil {
		t.Fatal("expected error when llm fails")
	}
}

func TestAgent_SystemPromptPassedThrough(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{llm.TextResponse("ok")},
	}
	a := newAgent(mock, noopTools{})

	_, _ = a.Run(context.Background(), "s1", "hi", "you are helpful")

	if mock.Requests[0].SystemPrompt != "you are helpful" {
		t.Fatalf("system prompt not forwarded: got %q", mock.Requests[0].SystemPrompt)
	}
}

func TestAgent_ToolDefsPassedThrough(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{llm.TextResponse("ok")},
	}
	tools := stubTools{
		defs: []llm.ToolDefinition{{Name: "my_tool", Description: "does stuff"}},
	}
	a := newAgent(mock, tools)

	_, _ = a.Run(context.Background(), "s1", "hi", "")

	if len(mock.Requests[0].Tools) != 1 || mock.Requests[0].Tools[0].Name != "my_tool" {
		t.Fatalf("tool definitions not forwarded: got %v", mock.Requests[0].Tools)
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	cases := []struct {
		soul, identity, want string
	}{
		{"soul", "identity", "soul\n\nidentity"},
		{"soul", "", "soul"},
		{"", "identity", "identity"},
		{"", "", ""},
	}
	for _, c := range cases {
		p := config.Personality{Soul: c.soul, Identity: c.identity}
		got := agent.BuildSystemPrompt(nil, p, "")
		if got != c.want {
			t.Errorf("BuildSystemPrompt(nil, %q, %q) = %q, want %q", c.soul, c.identity, got, c.want)
		}
	}
}

func TestHistory_AppendGetClear(t *testing.T) {
	h := agent.NewHistory()

	if msgs := h.Get("s1"); len(msgs) != 0 {
		t.Fatalf("expected empty, got %d messages", len(msgs))
	}

	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}})
	h.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "hi"}}})

	if msgs := h.Get("s1"); len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	h.Clear("s1")
	if msgs := h.Get("s1"); len(msgs) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(msgs))
	}
}

func TestHistory_GetReturnsCopy(t *testing.T) {
	h := agent.NewHistory()
	h.Append("s1", llm.Message{Role: llm.RoleUser})

	msgs := h.Get("s1")
	msgs[0].Role = llm.RoleAssistant // mutate the copy

	original := h.Get("s1")
	if original[0].Role != llm.RoleUser {
		t.Fatal("Get should return a copy, not a reference")
	}
}

func TestAgent_RunStream_DeliversDeltas(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{llm.TextResponse("Hello, world!")},
	}
	a := newAgent(mock, noopTools{})

	var deltas []string
	result, err := a.RunStream(context.Background(), "s1", "hi", "", func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello, world!" {
		t.Fatalf("expected result.Text %q, got %q", "Hello, world!", result.Text)
	}
	if len(deltas) == 0 {
		t.Fatal("expected at least one streaming delta")
	}
	joined := strings.Join(deltas, "")
	if joined != "Hello, world!" {
		t.Fatalf("expected joined deltas %q, got %q", "Hello, world!", joined)
	}
}

// clientOnly wraps a Client without exposing StreamingClient,
// used to test the non-streaming fallback path.
type clientOnly struct{ llm.Client }

func TestAgent_RunStream_FallsBackWithoutStreaming(t *testing.T) {
	mock := &llm.MockClient{Responses: []llm.Response{llm.TextResponse("ok")}}
	a := newAgent(clientOnly{mock}, noopTools{})

	called := false
	result, err := a.RunStream(context.Background(), "s1", "hi", "", func(_ string) {
		called = true
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("expected %q, got %q", "ok", result.Text)
	}
	if called {
		t.Fatal("handler should not be called when streaming is not supported")
	}
}

// contextCheckClient respects context cancellation before serving a response.
type contextCheckClient struct {
	resp llm.Response
}

func (c *contextCheckClient) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	select {
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	default:
		return c.resp, nil
	}
}

func TestAgent_ContextCancelled(t *testing.T) {
	client := &contextCheckClient{resp: llm.TextResponse("too late")}
	a := newAgent(client, noopTools{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run

	_, err := a.Run(ctx, "s1", "hello", "")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

func TestHistory_Lines_Empty(t *testing.T) {
	h := agent.NewHistory()
	if lines := h.Lines("s1"); lines != nil {
		t.Fatalf("expected nil for empty session, got %v", lines)
	}
}

func TestHistory_Lines_UserAndAssistant(t *testing.T) {
	h := agent.NewHistory()
	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}})
	h.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "hi there"}}})

	lines := h.Lines("s1")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "you: hello") {
		t.Errorf("line[0] should contain 'you: hello', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "assistant: hi there") {
		t.Errorf("line[1] should contain 'assistant: hi there', got %q", lines[1])
	}
}

func TestHistory_Lines_SkipsToolRoleMessages(t *testing.T) {
	h := agent.NewHistory()
	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "run it"}}})
	h.Append("s1", llm.Message{Role: llm.RoleTool, Content: []llm.ContentBlock{{Text: "tool output"}}})
	h.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "done"}}})

	lines := h.Lines("s1")
	if len(lines) != 2 {
		t.Fatalf("tool role should be skipped: expected 2 lines, got %d: %v", len(lines), lines)
	}
}

func TestHistory_Lines_ShowsToolCallsInAssistantMessages(t *testing.T) {
	h := agent.NewHistory()
	toolUse := &llm.ToolUse{ID: "tu1", Name: "current_time"}
	h.Append("s1", llm.Message{
		Role:    llm.RoleAssistant,
		Content: []llm.ContentBlock{{ToolUse: toolUse}},
	})

	lines := h.Lines("s1")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for tool use, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "[tool: current_time]") {
		t.Errorf("line should show tool call name, got %q", lines[0])
	}
}

func TestHistory_Lines_TextAndToolUseInSameMessage(t *testing.T) {
	h := agent.NewHistory()
	toolUse := &llm.ToolUse{ID: "tu1", Name: "bash"}
	h.Append("s1", llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentBlock{
			{Text: "let me check"},
			{ToolUse: toolUse},
		},
	})

	lines := h.Lines("s1")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (text + tool call), got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "let me check") {
		t.Errorf("first line should contain text, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "[tool: bash]") {
		t.Errorf("second line should show tool call, got %q", lines[1])
	}
}

// ensure errors package used (suppress unused import)
var _ = errors.New
