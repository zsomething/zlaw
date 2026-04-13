package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsomething/zlaw/internal/tools/builtin"
)

func TestWriteFile_basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	tool := builtin.WriteFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "content": "hello"})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") && !strings.Contains(out, "5 bytes") {
		t.Fatalf("unexpected output: %q", out)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Fatalf("file content mismatch: %q", string(data))
	}
}

func TestWriteFile_createsParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "out.txt")
	tool := builtin.WriteFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "content": "deep"})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "deep" {
		t.Fatalf("file content mismatch: %q", string(data))
	}
}

func TestWriteFile_overwritesExisting(t *testing.T) {
	path := writeTemp(t, "old content")
	tool := builtin.WriteFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "content": "new content"})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new content" {
		t.Fatalf("expected 'new content', got: %q", string(data))
	}
}

func TestWriteFile_missingPath(t *testing.T) {
	tool := builtin.WriteFile{}
	input, _ := json.Marshal(map[string]any{"content": "hello"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestWriteFile_schemaIsValidJSON(t *testing.T) {
	def := builtin.WriteFile{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
