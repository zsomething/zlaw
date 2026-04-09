package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/config"
)

func TestBuildPrefill_Empty(t *testing.T) {
	out, err := agent.BuildPrefill("/any", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("empty sources should return empty string, got: %q", out)
	}
}

func TestBuildPrefill_CWD(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"cwd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cwd:") {
		t.Fatalf("expected cwd: in output, got: %q", out)
	}
}

func TestBuildPrefill_Datetime(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"datetime"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "datetime:") {
		t.Fatalf("expected datetime: in output, got: %q", out)
	}
}

func TestBuildPrefill_File(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("test soul"), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := agent.BuildPrefill(dir, []string{"file:SOUL.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test soul") {
		t.Fatalf("expected file contents in output, got: %q", out)
	}
	if !strings.Contains(out, "file:SOUL.md") {
		t.Fatalf("expected file label in output, got: %q", out)
	}
}

func TestBuildPrefill_UnknownSource(t *testing.T) {
	_, err := agent.BuildPrefill("/any", []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestBuildPrefill_MultipleSourcesHeader(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"cwd", "datetime"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "[Session context]") {
		t.Fatalf("expected [Session context] header, got: %q", out)
	}
}

func TestBuildSystemPrompt_WithStickyBlocks(t *testing.T) {
	sticky := []agent.StickyBlock{
		{Name: "rules", Content: "always be concise"},
	}
	p := config.Personality{Soul: "you are helpful", Identity: ""}
	got := agent.BuildSystemPrompt(sticky, p)
	if !strings.HasPrefix(got, "always be concise") {
		t.Errorf("sticky block should be first, got: %q", got)
	}
	if !strings.Contains(got, "you are helpful") {
		t.Errorf("personality should be included, got: %q", got)
	}
}

func TestBuildSystemPrompt_NoSticky(t *testing.T) {
	p := config.Personality{Soul: "you are helpful", Identity: ""}
	got := agent.BuildSystemPrompt(nil, p)
	if got != "you are helpful" {
		t.Errorf("no sticky: want %q, got %q", "you are helpful", got)
	}
}

func TestBuildSystemSections_TwoSections(t *testing.T) {
	sticky := []agent.StickyBlock{
		{Name: "rules", Content: "always be concise"},
	}
	p := config.Personality{Soul: "you are helpful", Identity: "assistant context"}
	sections := agent.BuildSystemSections(sticky, p)
	if len(sections) != 2 {
		t.Fatalf("want 2 sections, got %d", len(sections))
	}
	// Section 1: sticky, with cache checkpoint
	if sections[0].Content != "always be concise" {
		t.Errorf("section 1 content = %q, want sticky text", sections[0].Content)
	}
	if !sections[0].CacheCheckpoint {
		t.Error("section 1 should have CacheCheckpoint=true")
	}
	// Section 2: personality, with cache checkpoint
	if !strings.Contains(sections[1].Content, "you are helpful") {
		t.Errorf("section 2 should contain soul, got: %q", sections[1].Content)
	}
	if !sections[1].CacheCheckpoint {
		t.Error("section 2 should have CacheCheckpoint=true")
	}
}

func TestBuildSystemSections_NoSticky(t *testing.T) {
	p := config.Personality{Soul: "helpful", Identity: ""}
	sections := agent.BuildSystemSections(nil, p)
	if len(sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(sections))
	}
	if sections[0].Content != "helpful" {
		t.Errorf("section content = %q, want %q", sections[0].Content, "helpful")
	}
}

func TestBuildSystemSections_Empty(t *testing.T) {
	sections := agent.BuildSystemSections(nil, config.Personality{})
	if len(sections) != 0 {
		t.Fatalf("want 0 sections, got %d", len(sections))
	}
}

// stubStore is a minimal MemoryStore backed by a slice, for testing.
type stubStore struct{ memories []agent.Memory }

func (s *stubStore) Save(m agent.Memory) error    { s.memories = append(s.memories, m); return nil }
func (s *stubStore) Delete(_ string) error        { return nil }
func (s *stubStore) List() ([]agent.Memory, error) { return s.memories, nil }
func (s *stubStore) Search(_ []string) ([]agent.Memory, error) { return s.memories, nil }

func TestBuildMemoriesSection_Format(t *testing.T) {
	store := &stubStore{memories: []agent.Memory{
		{ID: "a", Content: "user prefers Go", Tags: []string{"prefs"}},
		{ID: "b", Content: "project is Phase 1", Tags: []string{"project"}},
	}}
	out, err := agent.BuildMemoriesSection(store, 0)
	if err != nil {
		t.Fatalf("BuildMemoriesSection: %v", err)
	}
	if !strings.HasPrefix(out, "[Memories]\n") {
		t.Errorf("expected [Memories] header, got: %q", out)
	}
	if !strings.Contains(out, "user prefers Go #prefs") {
		t.Errorf("expected formatted memory line, got: %q", out)
	}
}

func TestBuildMemoriesSection_NilStore(t *testing.T) {
	out, err := agent.BuildMemoriesSection(nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("nil store should return empty string, got: %q", out)
	}
}

func TestBuildMemoriesSection_EmptyStore(t *testing.T) {
	store := &stubStore{}
	out, err := agent.BuildMemoriesSection(store, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("empty store should return empty string, got: %q", out)
	}
}

func TestBuildMemoriesSection_TokenBudget(t *testing.T) {
	// Create a store with a very long memory.
	longContent := strings.Repeat("x", 1000)
	store := &stubStore{memories: []agent.Memory{
		{ID: "long", Content: longContent},
		{ID: "short", Content: "short"},
	}}
	// Budget of 5 tokens = 20 chars — fits the short one but not the long one.
	out, err := agent.BuildMemoriesSection(store, 5)
	if err != nil {
		t.Fatalf("BuildMemoriesSection: %v", err)
	}
	if strings.Contains(out, longContent[:10]) {
		t.Errorf("long memory should be truncated, got: %q", out[:min(len(out), 80)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
