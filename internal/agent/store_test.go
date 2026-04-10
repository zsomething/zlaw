package agent_test

import (
	"testing"

	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
)

func TestJSONLFileStore_AppendLoad(t *testing.T) {
	dir := t.TempDir()
	store := agent.NewJSONLFileStore(dir)

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "hi there"}}},
	}

	for _, m := range msgs {
		if err := store.Append("sess1", m); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	loaded, err := store.Load("sess1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(loaded))
	}
	for i, m := range msgs {
		if loaded[i].Role != m.Role {
			t.Errorf("msg[%d].Role: want %q got %q", i, m.Role, loaded[i].Role)
		}
		if loaded[i].Content[0].Text != m.Content[0].Text {
			t.Errorf("msg[%d].Text: want %q got %q", i, m.Content[0].Text, loaded[i].Content[0].Text)
		}
	}
}

func TestJSONLFileStore_LoadMissing(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())
	msgs, err := store.Load("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error for missing session, got: %v", err)
	}
	if msgs != nil {
		t.Fatalf("expected nil messages for missing session, got %d", len(msgs))
	}
}

func TestJSONLFileStore_SessionsAreIsolated(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())

	_ = store.Append("a", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "from a"}}})
	_ = store.Append("b", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "from b"}}})

	a, _ := store.Load("a")
	b, _ := store.Load("b")

	if len(a) != 1 || a[0].Content[0].Text != "from a" {
		t.Fatalf("session a wrong: %v", a)
	}
	if len(b) != 1 || b[0].Content[0].Text != "from b" {
		t.Fatalf("session b wrong: %v", b)
	}
}

func TestJSONLFileStore_ToolUseRoundtrip(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())

	input := []byte(`{"key":"value"}`)
	msg := llm.Message{
		Role: llm.RoleAssistant,
		Content: []llm.ContentBlock{
			{ToolUse: &llm.ToolUse{ID: "tu-1", Name: "my_tool", Input: input}},
		},
	}

	_ = store.Append("s", msg)
	loaded, err := store.Load("s")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded))
	}
	tu := loaded[0].Content[0].ToolUse
	if tu == nil {
		t.Fatal("expected ToolUse, got nil")
	}
	if tu.ID != "tu-1" || tu.Name != "my_tool" {
		t.Fatalf("ToolUse fields wrong: %+v", tu)
	}
}

func TestJSONLFileStore_UpdateMetaRoundtrip(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())

	// UpdateMeta on a new session starts from zero value.
	if err := store.UpdateMeta("s1", func(m *agent.SessionMeta) {
		m.SessionID = "s1"
		m.Channel = "cli"
		m.MessageCount = 3
	}); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}

	meta, err := store.LoadMeta("s1")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.SessionID != "s1" {
		t.Errorf("session_id = %q, want %q", meta.SessionID, "s1")
	}
	if meta.Channel != "cli" {
		t.Errorf("channel = %q, want %q", meta.Channel, "cli")
	}
	if meta.MessageCount != 3 {
		t.Errorf("message_count = %d, want 3", meta.MessageCount)
	}
}

func TestJSONLFileStore_MetaLoadMissing(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())
	meta, err := store.LoadMeta("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error for missing meta, got: %v", err)
	}
	if !meta.CreatedAt.IsZero() {
		t.Error("expected zero CreatedAt for missing session")
	}
}

func TestHistory_MetaTracking(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())
	h := agent.NewHistoryWithStore(store, "test-channel")

	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "What is 2+2?"}}})
	h.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "4"}}})

	meta, err := store.LoadMeta("s1")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Channel != "test-channel" {
		t.Errorf("channel = %q, want %q", meta.Channel, "test-channel")
	}
	if meta.MessageCount != 2 {
		t.Errorf("message_count = %d, want 2", meta.MessageCount)
	}
	if meta.Title != "What is 2+2?" {
		t.Errorf("title = %q, want %q", meta.Title, "What is 2+2?")
	}
	if meta.CreatedAt.IsZero() {
		t.Error("created_at should be set")
	}
	if meta.UpdatedAt.IsZero() {
		t.Error("updated_at should be set")
	}
}

func TestJSONLFileStore_Archive(t *testing.T) {
	dir := t.TempDir()
	store := agent.NewJSONLFileStore(dir)

	msg := llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}}
	if err := store.Append("s1", msg); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.UpdateMeta("s1", func(m *agent.SessionMeta) { m.Channel = "cli" }); err != nil {
		t.Fatalf("UpdateMeta: %v", err)
	}

	if err := store.Archive("s1"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Active load should return nothing.
	msgs, err := store.Load("s1")
	if err != nil {
		t.Fatalf("Load after Archive: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after Archive, got %d", len(msgs))
	}

	// Archive again on missing file should not error.
	if err := store.Archive("s1"); err != nil {
		t.Fatalf("Archive of already-archived session: %v", err)
	}
}

func TestHistory_Clear_DoesNotReloadFromDisk(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())
	h := agent.NewHistoryWithStore(store, "test")

	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "old message"}}})

	h.Clear("s1")

	// Get must return empty — the file was archived and in-memory cache cleared.
	msgs := h.Get("s1")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after Clear, got %d", len(msgs))
	}

	// New History (simulates process restart) must also return empty.
	h2 := agent.NewHistoryWithStore(store, "test")
	msgs = h2.Get("s1")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after restart post-Clear, got %d", len(msgs))
	}
}

func TestHistory_RecordUsage(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())
	h := agent.NewHistoryWithStore(store, "cli")

	h.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}})
	h.RecordUsage("s1", llm.Usage{InputTokens: 100, OutputTokens: 50})
	h.RecordUsage("s1", llm.Usage{InputTokens: 200, OutputTokens: 75})

	meta, err := store.LoadMeta("s1")
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.TotalInputTokens != 300 {
		t.Errorf("total_input_tokens = %d, want 300", meta.TotalInputTokens)
	}
	if meta.TotalOutputTokens != 125 {
		t.Errorf("total_output_tokens = %d, want 125", meta.TotalOutputTokens)
	}
}

func TestHistory_WithStore_Persist(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())

	h1 := agent.NewHistoryWithStore(store, "")
	h1.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}})
	h1.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "world"}}})

	// New History with same store should reload from disk.
	h2 := agent.NewHistoryWithStore(store, "")
	msgs := h2.Get("s1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != "hello" || msgs[1].Content[0].Text != "world" {
		t.Fatalf("persisted messages wrong: %v", msgs)
	}
}
