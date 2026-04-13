package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/zsomething/zlaw/internal/tools/builtin"
)

func TestEditFile_basic(t *testing.T) {
	path := writeTemp(t, "hello world")
	tool := builtin.EditFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "old_string": "world", "new_string": "Go"})
	_, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello Go" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestEditFile_notFound(t *testing.T) {
	path := writeTemp(t, "hello world")
	tool := builtin.EditFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "old_string": "nope", "new_string": "x"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when old_string not found")
	}
}

func TestEditFile_ambiguous(t *testing.T) {
	path := writeTemp(t, "foo foo foo")
	tool := builtin.EditFile{}

	input, _ := json.Marshal(map[string]any{"path": path, "old_string": "foo", "new_string": "bar"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}
}

func TestEditFile_missingPath(t *testing.T) {
	tool := builtin.EditFile{}
	input, _ := json.Marshal(map[string]any{"old_string": "x", "new_string": "y"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestEditFile_missingOldString(t *testing.T) {
	path := writeTemp(t, "content")
	tool := builtin.EditFile{}
	input, _ := json.Marshal(map[string]any{"path": path, "new_string": "y"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing old_string")
	}
}

func TestEditFile_fileNotFound(t *testing.T) {
	tool := builtin.EditFile{}
	input, _ := json.Marshal(map[string]any{"path": "/tmp/does_not_exist_zlaw.txt", "old_string": "x", "new_string": "y"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestEditFile_schemaIsValidJSON(t *testing.T) {
	def := builtin.EditFile{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
