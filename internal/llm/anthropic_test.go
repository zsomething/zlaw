package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/chickenzord/zlaw/internal/llm"
)

func anthropicTextResponse(text string) map[string]interface{} {
	return map[string]interface{}{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
		"stop_reason": "end_turn",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	}
}

func anthropicToolUseResponse(id, name, argsJSON string) map[string]interface{} {
	var input interface{}
	_ = json.Unmarshal([]byte(argsJSON), &input)
	return map[string]interface{}{
		"id":   "msg_test",
		"type": "message",
		"role": "assistant",
		"content": []map[string]interface{}{
			{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
				"input": input,
			},
		},
		"stop_reason": "tool_use",
		"usage": map[string]interface{}{
			"input_tokens":  10,
			"output_tokens": 5,
		},
	}
}

func newAnthropicTestClient(t *testing.T, handler http.HandlerFunc) llm.Client {
	t.Helper()
	srv := newTestServer(t, handler)
	client, err := llm.NewAnthropicClient(llm.AnthropicConfig{
		BaseURL:     srv.URL,
		TokenSource: staticSource{"test-key"},
		Model:       "claude-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAnthropic_TextResponse(t *testing.T) {
	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q, want %q", r.Header.Get("x-api-key"), "test-key")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("anthropic-version header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("Hello from Anthropic!"))
	})

	resp, err := client.Complete(context.Background(), llm.Request{
		SystemPrompt: "You are helpful.",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.TextContent() != "Hello from Anthropic!" {
		t.Errorf("text = %q", resp.Message.TextContent())
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestAnthropic_ToolUseResponse(t *testing.T) {
	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicToolUseResponse("toolu_1", "current_time", `{}`))
	})

	resp, err := client.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "what time is it?"}}},
		},
		Tools: []llm.ToolDefinition{
			{Name: "current_time", Description: "Returns current time"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	uses := resp.Message.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool uses = %d, want 1", len(uses))
	}
	if uses[0].ID != "toolu_1" || uses[0].Name != "current_time" {
		t.Errorf("tool use = %+v", uses[0])
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", resp.StopReason)
	}
}

func TestAnthropic_RequestFormat(t *testing.T) {
	var capturedBody map[string]interface{}
	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		SystemPrompt: "be helpful",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}},
			{
				Role: llm.RoleAssistant,
				Content: []llm.ContentBlock{
					{ToolUse: &llm.ToolUse{ID: "tu1", Name: "mytool", Input: []byte(`{"x":1}`)}},
				},
			},
			{
				Role: llm.RoleTool,
				Content: []llm.ContentBlock{
					{ToolResult: &llm.ToolResult{ToolUseID: "tu1", Content: "result text"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if capturedBody["system"] != "be helpful" {
		t.Errorf("system = %v, want %q", capturedBody["system"], "be helpful")
	}

	msgs, ok := capturedBody["messages"].([]interface{})
	if !ok || len(msgs) != 3 {
		t.Fatalf("messages count = %d, want 3; body = %v", len(msgs), capturedBody["messages"])
	}

	// Third message (tool result) must be role=user with tool_result content block.
	toolResultMsg := msgs[2].(map[string]interface{})
	if toolResultMsg["role"] != "user" {
		t.Errorf("tool result message role = %v, want user", toolResultMsg["role"])
	}
	trContent := toolResultMsg["content"].([]interface{})
	if len(trContent) == 0 {
		t.Fatal("tool result content is empty")
	}
	trBlock := trContent[0].(map[string]interface{})
	if trBlock["type"] != "tool_result" {
		t.Errorf("content block type = %v, want tool_result", trBlock["type"])
	}
	if trBlock["tool_use_id"] != "tu1" {
		t.Errorf("tool_use_id = %v, want tu1", trBlock["tool_use_id"])
	}
}

func TestAnthropic_RateLimit(t *testing.T) {
	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`)
	})

	_, err := client.Complete(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !isRateLimit(err) {
		t.Errorf("expected ErrRateLimit in chain, got: %v", err)
	}
}

func TestAnthropic_Streaming(t *testing.T) {
	sseBody := "" +
		"event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	})

	sc, ok := client.(llm.StreamingClient)
	if !ok {
		t.Fatal("client does not implement StreamingClient")
	}

	var received []string
	resp, err := sc.CompleteStream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	}, func(delta string) {
		received = append(received, delta)
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Message.TextContent() != "Hello, world!" {
		t.Errorf("text = %q, want %q", resp.Message.TextContent(), "Hello, world!")
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if len(received) != 2 {
		t.Errorf("stream deltas = %d, want 2: %v", len(received), received)
	}
}

func newAnthropicTestClientWithCaching(t *testing.T, promptCaching bool, handler http.HandlerFunc) llm.Client {
	t.Helper()
	srv := newTestServer(t, handler)
	client, err := llm.NewAnthropicClient(llm.AnthropicConfig{
		BaseURL:       srv.URL,
		TokenSource:   staticSource{"test-key"},
		Model:         "claude-test",
		PromptCaching: promptCaching,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAnthropic_PromptCaching_SystemAsArray(t *testing.T) {
	var capturedBody map[string]interface{}
	var capturedBetaHeader string
	client := newAnthropicTestClientWithCaching(t, true, func(w http.ResponseWriter, r *http.Request) {
		capturedBetaHeader = r.Header.Get("anthropic-beta")
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		SystemPrompt: "be helpful",
		Messages:     []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Beta header must be present.
	if capturedBetaHeader != "prompt-caching-2024-07-31" {
		t.Errorf("anthropic-beta = %q, want prompt-caching-2024-07-31", capturedBetaHeader)
	}

	// System field must be an array with cache_control.
	sysRaw, ok := capturedBody["system"].([]interface{})
	if !ok {
		t.Fatalf("system must be an array when prompt caching enabled, got: %T %v", capturedBody["system"], capturedBody["system"])
	}
	if len(sysRaw) != 1 {
		t.Fatalf("system array length = %d, want 1", len(sysRaw))
	}
	block := sysRaw[0].(map[string]interface{})
	if block["type"] != "text" {
		t.Errorf("system block type = %v, want text", block["type"])
	}
	if block["text"] != "be helpful" {
		t.Errorf("system block text = %v, want 'be helpful'", block["text"])
	}
	cc, ok := block["cache_control"].(map[string]interface{})
	if !ok {
		t.Fatal("system block missing cache_control")
	}
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control.type = %v, want ephemeral", cc["type"])
	}
}

func TestAnthropic_PromptCaching_DisabledSystemAsString(t *testing.T) {
	var capturedBody map[string]interface{}
	var capturedBetaHeader string
	client := newAnthropicTestClientWithCaching(t, false, func(w http.ResponseWriter, r *http.Request) {
		capturedBetaHeader = r.Header.Get("anthropic-beta")
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		SystemPrompt: "be helpful",
		Messages:     []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Beta header must NOT be present.
	if capturedBetaHeader != "" {
		t.Errorf("anthropic-beta header should not be set when caching disabled, got: %q", capturedBetaHeader)
	}

	// System field must be a plain string.
	if capturedBody["system"] != "be helpful" {
		t.Errorf("system = %v, want plain string 'be helpful'", capturedBody["system"])
	}
}

func TestAnthropic_SystemSections_TwoCheckpoints(t *testing.T) {
	var capturedBody map[string]interface{}
	var capturedBetaHeader string
	client := newAnthropicTestClientWithCaching(t, true, func(w http.ResponseWriter, r *http.Request) {
		capturedBetaHeader = r.Header.Get("anthropic-beta")
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		SystemSections: []llm.SystemSection{
			{Content: "sticky rules", CacheCheckpoint: true},
			{Content: "personality", CacheCheckpoint: true},
		},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if capturedBetaHeader != "prompt-caching-2024-07-31" {
		t.Errorf("anthropic-beta = %q, want prompt-caching-2024-07-31", capturedBetaHeader)
	}

	sysRaw, ok := capturedBody["system"].([]interface{})
	if !ok {
		t.Fatalf("system must be an array, got: %T %v", capturedBody["system"], capturedBody["system"])
	}
	if len(sysRaw) != 2 {
		t.Fatalf("system array length = %d, want 2", len(sysRaw))
	}

	// Both blocks should have cache_control.
	for i, raw := range sysRaw {
		blk := raw.(map[string]interface{})
		cc, ok := blk["cache_control"].(map[string]interface{})
		if !ok {
			t.Errorf("block %d missing cache_control", i)
			continue
		}
		if cc["type"] != "ephemeral" {
			t.Errorf("block %d cache_control.type = %v, want ephemeral", i, cc["type"])
		}
	}
	if sysRaw[0].(map[string]interface{})["text"] != "sticky rules" {
		t.Errorf("block 0 text = %v, want 'sticky rules'", sysRaw[0].(map[string]interface{})["text"])
	}
	if sysRaw[1].(map[string]interface{})["text"] != "personality" {
		t.Errorf("block 1 text = %v, want 'personality'", sysRaw[1].(map[string]interface{})["text"])
	}
}

func TestAnthropic_SystemSections_NoCachingWhenDisabled(t *testing.T) {
	var capturedBody map[string]interface{}
	client := newAnthropicTestClientWithCaching(t, false, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		SystemSections: []llm.SystemSection{
			{Content: "sticky rules", CacheCheckpoint: true},
			{Content: "personality", CacheCheckpoint: true},
		},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	sysRaw, ok := capturedBody["system"].([]interface{})
	if !ok {
		t.Fatalf("system must be an array, got: %T %v", capturedBody["system"], capturedBody["system"])
	}
	// Blocks should NOT have cache_control when caching is disabled.
	for i, raw := range sysRaw {
		blk := raw.(map[string]interface{})
		if _, hasCacheControl := blk["cache_control"]; hasCacheControl {
			t.Errorf("block %d should not have cache_control when caching disabled", i)
		}
	}
}

func TestAnthropic_PromptCaching_NoHeaderWhenNoSystem(t *testing.T) {
	var capturedBetaHeader string
	client := newAnthropicTestClientWithCaching(t, true, func(w http.ResponseWriter, r *http.Request) {
		capturedBetaHeader = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicTextResponse("ok"))
	})

	_, err := client.Complete(context.Background(), llm.Request{
		// No SystemPrompt
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Beta header must NOT be present when there is no system prompt.
	if capturedBetaHeader != "" {
		t.Errorf("anthropic-beta header should not be set when system prompt is empty, got: %q", capturedBetaHeader)
	}
}

func TestAnthropic_StreamingToolUse(t *testing.T) {
	sseBody := "" +
		"event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":15,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"current_time"}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":8}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	client := newAnthropicTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody)
	})

	sc := client.(llm.StreamingClient)
	resp, err := sc.CompleteStream(context.Background(), llm.Request{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "what time?"}}}},
	}, func(delta string) {})
	if err != nil {
		t.Fatal(err)
	}

	uses := resp.Message.ToolUses()
	if len(uses) != 1 {
		t.Fatalf("tool uses = %d, want 1", len(uses))
	}
	if uses[0].ID != "toolu_1" || uses[0].Name != "current_time" {
		t.Errorf("tool use = %+v", uses[0])
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", resp.StopReason)
	}
}
