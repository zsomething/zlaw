package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zsomething/zlaw/internal/credentials"
	"github.com/zsomething/zlaw/internal/llm"
)

// staticSource is an inline TokenSource for tests.
type staticSource struct{ token string }

func (s staticSource) Token(_ context.Context) (string, error) { return s.token, nil }

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func openAIResponse(content string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "cmpl-test",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}
}

func openAIToolCallResponse(id, name, args string) map[string]interface{} {
	return map[string]interface{}{
		"id":      "cmpl-test",
		"object":  "chat.completion",
		"created": 1234567890,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]interface{}{
						{
							"id":   id,
							"type": "function",
							"function": map[string]interface{}{
								"name":      name,
								"arguments": args,
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		},
	}
}

func TestOpenAICompat_TextResponse(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIResponse("Hello, world!"))
	})

	client, err := llm.NewOpenAICompatClient(llm.OpenAICompatConfig{
		BaseURL:     srv.URL,
		TokenSource: staticSource{"test-token"},
		Model:       "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Complete(context.Background(), llm.Request{
		SystemPrompt: "You are helpful.",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Message.TextContent() != "Hello, world!" {
		t.Errorf("text = %q, want %q", resp.Message.TextContent(), "Hello, world!")
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want %q", resp.StopReason, "end_turn")
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestOpenAICompat_ToolCallResponse(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIToolCallResponse("call-1", "current_time", `{}`))
	})

	client, err := llm.NewOpenAICompatClient(llm.OpenAICompatConfig{
		BaseURL:     srv.URL,
		TokenSource: staticSource{"tok"},
		Model:       "m",
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "what time is it?"}}},
		},
		Tools: []llm.ToolDefinition{
			{Name: "current_time", Description: "Returns current UTC time"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	uses := resp.Message.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool uses = %d, want 1", len(uses))
	}
	if uses[0].Name != "current_time" || uses[0].ID != "call-1" {
		t.Errorf("tool use = %+v", uses[0])
	}
}

func TestOpenAICompat_RateLimit(t *testing.T) {
	srv := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"requests"}}`))
	})

	client, err := llm.NewOpenAICompatClient(llm.OpenAICompatConfig{
		BaseURL:     srv.URL,
		TokenSource: staticSource{"tok"},
		Model:       "m",
		MaxRetries:  0,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should wrap ErrRateLimit
	if !isRateLimit(err) {
		t.Errorf("expected ErrRateLimit in error chain, got: %v", err)
	}
}

func isRateLimit(err error) bool {
	return errors.Is(err, llm.ErrRateLimit)
}

var _ = errors.New // use errors package

func TestMock_TextResponse(t *testing.T) {
	mock := &llm.MockClient{
		Responses: []llm.Response{llm.TextResponse("pong")},
	}

	resp, err := mock.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "ping"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.TextContent() != "pong" {
		t.Errorf("text = %q", resp.Message.TextContent())
	}
	if len(mock.Requests) != 1 {
		t.Errorf("recorded %d requests, want 1", len(mock.Requests))
	}
}

func TestMock_Exhausted(t *testing.T) {
	mock := &llm.MockClient{}
	_, err := mock.Complete(context.Background(), llm.Request{})
	if err == nil {
		t.Error("expected error when responses exhausted")
	}
}

// Ensure credentials package compiles
var _ = credentials.CredentialProfile{}
var _ staticSource
