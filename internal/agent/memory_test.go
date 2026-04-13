package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/agent"
)

func newTestStore(t *testing.T) *agent.MarkdownFileStore {
	t.Helper()
	dir := t.TempDir()
	return agent.NewMarkdownFileStore(dir)
}

func TestMarkdownFileStore_SaveAndList(t *testing.T) {
	store := newTestStore(t)

	m := agent.Memory{
		ID:      "mem-001",
		Tags:    []string{"go", "testing"},
		Content: "Go tests should be table-driven.",
	}
	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(all))
	}
	got := all[0]
	if got.ID != m.ID {
		t.Errorf("ID: got %q, want %q", got.ID, m.ID)
	}
	if got.Content != m.Content {
		t.Errorf("Content: got %q, want %q", got.Content, m.Content)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "go" || got.Tags[1] != "testing" {
		t.Errorf("Tags: got %v", got.Tags)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps should be set by Save")
	}
}

func TestMarkdownFileStore_SaveOverwrites(t *testing.T) {
	store := newTestStore(t)

	m := agent.Memory{ID: "mem-002", Content: "original"}
	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	m.Content = "updated"
	if err := store.Save(m); err != nil {
		t.Fatalf("Save (overwrite): %v", err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 memory after overwrite, got %d", len(all))
	}
	if all[0].Content != "updated" {
		t.Errorf("Content after overwrite: %q", all[0].Content)
	}
}

func TestMarkdownFileStore_PreservesCreatedAt(t *testing.T) {
	store := newTestStore(t)

	original := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := agent.Memory{ID: "mem-003", CreatedAt: original, Content: "hello"}
	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !all[0].CreatedAt.Equal(original) {
		t.Errorf("CreatedAt: got %v, want %v", all[0].CreatedAt, original)
	}
}

func TestMarkdownFileStore_Delete(t *testing.T) {
	store := newTestStore(t)

	m := agent.Memory{ID: "mem-004", Content: "to delete"}
	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete("mem-004"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	all, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 memories after delete, got %d", len(all))
	}
}

func TestMarkdownFileStore_DeleteNonExistent(t *testing.T) {
	store := newTestStore(t)
	// Should not error when file does not exist.
	if err := store.Delete("no-such-id"); err != nil {
		t.Errorf("Delete non-existent: %v", err)
	}
}

func TestMarkdownFileStore_Search_ByContent(t *testing.T) {
	store := newTestStore(t)

	memories := []agent.Memory{
		{ID: "a", Content: "Go is great for systems programming"},
		{ID: "b", Content: "Python is great for data science"},
		{ID: "c", Content: "Rust is great for safety"},
	}
	for _, m := range memories {
		if err := store.Save(m); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := store.Search([]string{"python"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "b" {
		t.Errorf("Search: got %v", results)
	}
}

func TestMarkdownFileStore_Search_ByTag(t *testing.T) {
	store := newTestStore(t)

	memories := []agent.Memory{
		{ID: "x", Tags: []string{"backend", "go"}, Content: "service A"},
		{ID: "y", Tags: []string{"frontend", "react"}, Content: "service B"},
	}
	for _, m := range memories {
		if err := store.Save(m); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := store.Search([]string{"go"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "x" {
		t.Errorf("Search by tag: got %v", results)
	}
}

func TestMarkdownFileStore_Search_EmptyKeywords(t *testing.T) {
	store := newTestStore(t)

	for _, id := range []string{"p", "q", "r"} {
		if err := store.Save(agent.Memory{ID: id, Content: id}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := store.Search(nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Search empty keywords: expected 3, got %d", len(results))
	}
}

func TestMarkdownFileStore_ListEmpty(t *testing.T) {
	store := newTestStore(t)
	all, err := store.List()
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty list, got %d", len(all))
	}
}

func TestMarkdownFileStore_FileIsHumanReadable(t *testing.T) {
	dir := t.TempDir()
	store := agent.NewMarkdownFileStore(dir)

	m := agent.Memory{
		ID:      "human-test",
		Tags:    []string{"example"},
		Content: "This should be readable by humans.",
	}
	if err := store.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "human-test.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	s := string(raw)
	if !strings.HasPrefix(s, "---\n") {
		t.Error("file should start with ---")
	}
	if !strings.Contains(s, "id: human-test") {
		t.Error("file should contain id frontmatter")
	}
	if !strings.Contains(s, "This should be readable by humans.") {
		t.Error("file should contain content")
	}
}
