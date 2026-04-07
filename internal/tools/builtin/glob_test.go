package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := []string{
		"a.go",
		"b.go",
		"sub/c.go",
		"sub/d.txt",
		"sub/inner/e.go",
	}
	for _, f := range files {
		full := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGlob_simplePattern(t *testing.T) {
	root := makeTree(t)
	tool := builtin.Glob{}

	input, _ := json.Marshal(map[string]any{"pattern": "*.go", "dir": root})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(lines), lines)
	}
}

func TestGlob_doublestar(t *testing.T) {
	root := makeTree(t)
	tool := builtin.Glob{}

	input, _ := json.Marshal(map[string]any{"pattern": "**/*.go", "dir": root})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// a.go, b.go, sub/c.go, sub/inner/e.go
	if len(lines) != 4 {
		t.Fatalf("expected 4 matches, got %d: %v", len(lines), lines)
	}
}

func TestGlob_noMatch(t *testing.T) {
	root := makeTree(t)
	tool := builtin.Glob{}

	input, _ := json.Marshal(map[string]any{"pattern": "**/*.rs", "dir": root})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got: %q", out)
	}
}

func TestGlob_missingPattern(t *testing.T) {
	tool := builtin.Glob{}
	input, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
}

func TestGlob_schemaIsValidJSON(t *testing.T) {
	def := builtin.Glob{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
