package builtin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

func TestGrep_basic(t *testing.T) {
	path := writeTemp(t, "hello world\nfoo bar\nhello again\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "hello", "paths": []string{path}})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(lines), lines)
	}
}

func TestGrep_caseInsensitive(t *testing.T) {
	path := writeTemp(t, "Hello World\nfoo bar\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "hello", "paths": []string{path}, "case_insensitive": true})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Hello World") {
		t.Fatalf("expected case-insensitive match, got: %q", out)
	}
}

func TestGrep_noMatch(t *testing.T) {
	path := writeTemp(t, "hello world\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "zzz", "paths": []string{path}})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got: %q", out)
	}
}

func TestGrep_multipleFiles(t *testing.T) {
	p1 := writeTemp(t, "match here\n")
	p2 := writeTemp(t, "nothing\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "match", "paths": []string{p1, p2}})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 match, got %d: %v", len(lines), lines)
	}
}

func TestGrep_lineNumbers(t *testing.T) {
	path := writeTemp(t, "line1\nline2\nmatch\nline4\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "match", "paths": []string{path}})
	out, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, ":3:match") {
		t.Fatalf("expected line number 3, got: %q", out)
	}
}

func TestGrep_invalidPattern(t *testing.T) {
	path := writeTemp(t, "hello\n")
	tool := builtin.GrepFiles{}

	input, _ := json.Marshal(map[string]any{"pattern": "[invalid", "paths": []string{path}})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestGrep_missingPattern(t *testing.T) {
	tool := builtin.GrepFiles{}
	input, _ := json.Marshal(map[string]any{"paths": []string{"/tmp/x"}})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing pattern")
	}
}

func TestGrep_missingPaths(t *testing.T) {
	tool := builtin.GrepFiles{}
	input, _ := json.Marshal(map[string]any{"pattern": "foo"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for missing paths")
	}
}

func TestGrep_schemaIsValidJSON(t *testing.T) {
	def := builtin.GrepFiles{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
