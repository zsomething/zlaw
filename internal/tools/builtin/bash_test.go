package builtin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

func TestBash_basic(t *testing.T) {
	tool := builtin.Bash{}
	input, _ := json.Marshal(map[string]any{"command": "echo hello"})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected 'hello' in output, got: %q", out)
	}
	if !strings.Contains(out, "exit_code: 0") {
		t.Fatalf("expected exit_code: 0, got: %q", out)
	}
}

func TestBash_nonZeroExit(t *testing.T) {
	tool := builtin.Bash{}
	input, _ := json.Marshal(map[string]any{"command": "exit 42"})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "exit_code: 42") {
		t.Fatalf("expected exit_code: 42, got: %q", out)
	}
}

func TestBash_stderr(t *testing.T) {
	tool := builtin.Bash{}
	input, _ := json.Marshal(map[string]any{"command": "echo error >&2"})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "stderr:") || !strings.Contains(out, "error") {
		t.Fatalf("expected stderr section, got: %q", out)
	}
}

func TestBash_workingDir(t *testing.T) {
	dir := t.TempDir()
	tool := builtin.Bash{}
	input, _ := json.Marshal(map[string]any{"command": "pwd", "working_dir": dir})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, dir) {
		t.Fatalf("expected working dir in output, got: %q", out)
	}
}

func TestBash_missingCommand(t *testing.T) {
	tool := builtin.Bash{}
	input, _ := json.Marshal(map[string]any{})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestBash_schemaIsValidJSON(t *testing.T) {
	def := builtin.Bash{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
