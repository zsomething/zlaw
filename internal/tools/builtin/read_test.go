package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsomething/zlaw/internal/tools/builtin"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "read_test_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func TestReadFile_fullFile(t *testing.T) {
	path := writeTemp(t, "line1\nline2\nline3\n")
	tool := builtin.ReadFile{}

	input, _ := json.Marshal(map[string]any{"path": path})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

func TestReadFile_withOffset(t *testing.T) {
	path := writeTemp(t, "line1\nline2\nline3\n")
	tool := builtin.ReadFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "offset": 1})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" || out[:5] != "line2" {
		t.Fatalf("expected output starting with line2, got: %q", out)
	}
}

func TestReadFile_withLimit(t *testing.T) {
	path := writeTemp(t, "line1\nline2\nline3\n")
	tool := builtin.ReadFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "limit": 1})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "line1" {
		t.Fatalf("expected exactly 'line1', got: %q", out)
	}
}

func TestReadFile_notFound(t *testing.T) {
	tool := builtin.ReadFile{}
	path := filepath.Join(t.TempDir(), "does_not_exist.txt")

	input, _ := json.Marshal(map[string]any{"path": path})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFile_missingPath(t *testing.T) {
	tool := builtin.ReadFile{}

	input, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestReadFile_schemaIsValidJSON(t *testing.T) {
	def := builtin.ReadFile{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}

func TestReadFile_offsetBeyondFile(t *testing.T) {
	path := writeTemp(t, "line1\n")
	tool := builtin.ReadFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "offset": 100})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got: %q", out)
	}
}
