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

func TestHistory_WithStore_Persist(t *testing.T) {
	store := agent.NewJSONLFileStore(t.TempDir())

	h1 := agent.NewHistoryWithStore(store)
	h1.Append("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}})
	h1.Append("s1", llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "world"}}})

	// New History with same store should reload from disk.
	h2 := agent.NewHistoryWithStore(store)
	msgs := h2.Get("s1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != "hello" || msgs[1].Content[0].Text != "world" {
		t.Fatalf("persisted messages wrong: %v", msgs)
	}
}
