package builtin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/tools/builtin"
)

// stubMemoryStore is an in-memory MemoryStore for testing.
type stubMemoryStore struct {
	data map[string]agent.Memory
}

func newStubMemoryStore() *stubMemoryStore {
	return &stubMemoryStore{data: make(map[string]agent.Memory)}
}

func (s *stubMemoryStore) Save(m agent.Memory) error {
	s.data[m.ID] = m
	return nil
}

func (s *stubMemoryStore) Delete(id string) error {
	delete(s.data, id)
	return nil
}

func (s *stubMemoryStore) List() ([]agent.Memory, error) {
	out := make([]agent.Memory, 0, len(s.data))
	for _, m := range s.data {
		out = append(out, m)
	}
	return out, nil
}

func (s *stubMemoryStore) Search(keywords []string) ([]agent.Memory, error) {
	all, _ := s.List()
	if len(keywords) == 0 {
		return all, nil
	}
	var results []agent.Memory
	for _, m := range all {
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(m.Content), strings.ToLower(kw)) {
				results = append(results, m)
				break
			}
			for _, tag := range m.Tags {
				if strings.Contains(strings.ToLower(tag), strings.ToLower(kw)) {
					results = append(results, m)
					break
				}
			}
		}
	}
	return results, nil
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestMemorySave_Basic(t *testing.T) {
	store := newStubMemoryStore()
	tool := builtin.MemorySave{Store: store}

	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"id":      "pref-lang",
		"content": "User prefers Go over Python.",
		"tags":    []string{"prefs"},
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "pref-lang" {
		t.Errorf("expected ID in result, got %q", result)
	}
	if _, ok := store.data["pref-lang"]; !ok {
		t.Error("memory not stored")
	}
}

func TestMemorySave_RequiresID(t *testing.T) {
	store := newStubMemoryStore()
	tool := builtin.MemorySave{Store: store}

	_, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"content": "some content",
	}))
	if err == nil {
		t.Error("expected error for missing id")
	}
}

func TestMemorySave_RequiresContent(t *testing.T) {
	store := newStubMemoryStore()
	tool := builtin.MemorySave{Store: store}

	_, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"id": "x",
	}))
	if err == nil {
		t.Error("expected error for missing content")
	}
}

func TestMemoryRecall_MatchesContent(t *testing.T) {
	store := newStubMemoryStore()
	_ = store.Save(agent.Memory{ID: "a", Content: "Go is great"})
	_ = store.Save(agent.Memory{ID: "b", Content: "Python is fine"})

	tool := builtin.MemoryRecall{Store: store}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"query": "Python",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Python is fine") {
		t.Errorf("result missing expected memory: %q", result)
	}
	if strings.Contains(result, "Go is great") {
		t.Errorf("result contains non-matching memory: %q", result)
	}
}

func TestMemoryRecall_EmptyQueryReturnsAll(t *testing.T) {
	store := newStubMemoryStore()
	_ = store.Save(agent.Memory{ID: "x", Content: "first"})
	_ = store.Save(agent.Memory{ID: "y", Content: "second"})

	tool := builtin.MemoryRecall{Store: store}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "first") || !strings.Contains(result, "second") {
		t.Errorf("expected both memories in result: %q", result)
	}
}

func TestMemoryRecall_NoMatchReturnsMessage(t *testing.T) {
	store := newStubMemoryStore()
	tool := builtin.MemoryRecall{Store: store}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"query": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "no memories") {
		t.Errorf("expected no-memories message, got %q", result)
	}
}

func TestMemoryDelete_RemovesMemory(t *testing.T) {
	store := newStubMemoryStore()
	_ = store.Save(agent.Memory{ID: "del-me", Content: "bye"})

	tool := builtin.MemoryDelete{Store: store}
	result, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{
		"id": "del-me",
	}))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "deleted" {
		t.Errorf("unexpected result %q", result)
	}
	if _, ok := store.data["del-me"]; ok {
		t.Error("memory still present after delete")
	}
}

func TestMemoryDelete_RequiresID(t *testing.T) {
	store := newStubMemoryStore()
	tool := builtin.MemoryDelete{Store: store}
	_, err := tool.Execute(context.Background(), mustJSON(t, map[string]any{}))
	if err == nil {
		t.Error("expected error for missing id")
	}
}

func TestMemoryTools_Definitions(t *testing.T) {
	store := newStubMemoryStore()
	tools := []interface {
		Definition() interface{ GetName() string }
	}{}
	_ = tools

	names := []string{
		builtin.MemorySave{Store: store}.Definition().Name,
		builtin.MemoryRecall{Store: store}.Definition().Name,
		builtin.MemoryDelete{Store: store}.Definition().Name,
	}
	want := []string{"memory_save", "memory_recall", "memory_delete"}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("tool[%d]: got %q, want %q", i, name, want[i])
		}
	}
}
