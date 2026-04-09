package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

// slowTool sleeps for the given duration then returns its name as the result.
type slowTool struct {
	name  string
	delay time.Duration
}

func (s slowTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{Name: s.name, InputSchema: json.RawMessage(`{}`)}
}

func (s slowTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	time.Sleep(s.delay)
	return s.name, nil
}

func TestRegistry_Execute_knownTool(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})

	call := llm.ToolUse{ID: "call-1", Name: "current_time", Input: []byte("{}")}
	res := r.Execute(context.Background(), call)

	if res.IsError {
		t.Fatalf("expected no error, got: %s", res.Content)
	}
	if res.Content == "" {
		t.Fatal("expected non-empty time string")
	}
	if res.ToolUseID != "call-1" {
		t.Fatalf("ToolUseID mismatch: got %q", res.ToolUseID)
	}
}

func TestRegistry_Execute_unknownTool(t *testing.T) {
	r := tools.NewRegistry()

	call := llm.ToolUse{ID: "call-2", Name: "does_not_exist", Input: []byte("{}")}
	res := r.Execute(context.Background(), call)

	if !res.IsError {
		t.Fatal("expected IsError=true for unknown tool")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "current_time" {
		t.Fatalf("unexpected tool name: %q", defs[0].Name)
	}
}

func TestRegistry_ExecuteAll(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})

	calls := []llm.ToolUse{
		{ID: "a", Name: "current_time", Input: []byte("{}")},
		{ID: "b", Name: "missing", Input: []byte("{}")},
	}
	results := r.ExecuteAll(context.Background(), calls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].IsError {
		t.Errorf("result[0] should succeed")
	}
	if !results[1].IsError {
		t.Errorf("result[1] should be an error")
	}
}

func TestRegistry_ExecuteAll_preservesOrder(t *testing.T) {
	r := tools.NewRegistry()
	// "slow" finishes last but must appear first in results.
	r.Register(slowTool{name: "slow", delay: 50 * time.Millisecond})
	r.Register(slowTool{name: "fast", delay: 0})

	calls := []llm.ToolUse{
		{ID: "1", Name: "slow", Input: []byte("{}")},
		{ID: "2", Name: "fast", Input: []byte("{}")},
	}
	results := r.ExecuteAll(context.Background(), calls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Content != "slow" {
		t.Errorf("results[0] should be 'slow', got %q", results[0].Content)
	}
	if results[1].Content != "fast" {
		t.Errorf("results[1] should be 'fast', got %q", results[1].Content)
	}
}

func TestRegistry_ExecuteAll_runsInParallel(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(slowTool{name: "slow_a", delay: 50 * time.Millisecond})
	r.Register(slowTool{name: "slow_b", delay: 50 * time.Millisecond})

	calls := []llm.ToolUse{
		{ID: "1", Name: "slow_a", Input: []byte("{}")},
		{ID: "2", Name: "slow_b", Input: []byte("{}")},
	}

	start := time.Now()
	r.ExecuteAll(context.Background(), calls)
	elapsed := time.Since(start)

	// Sequential would take ~100ms; parallel should complete in ~50ms.
	// Allow generous headroom for slow CI.
	if elapsed > 90*time.Millisecond {
		t.Errorf("ExecuteAll took %v; expected concurrent execution (~50ms)", elapsed)
	}
}

func TestRegistry_DuplicateRegister_panics(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})

	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	r.Register(builtin.CurrentTime{})
}

func TestRegistry_Allowlist_filtersDefinitions(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})
	r.Register(builtin.ReadFile{})

	r.SetAllowlist([]string{"current_time"})
	defs := r.Definitions()

	if len(defs) != 1 {
		t.Fatalf("expected 1 definition after allowlist, got %d", len(defs))
	}
	if defs[0].Name != "current_time" {
		t.Fatalf("unexpected tool in definitions: %q", defs[0].Name)
	}
}

func TestRegistry_Allowlist_blocksExecution(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})
	r.Register(builtin.ReadFile{})

	r.SetAllowlist([]string{"current_time"})
	call := llm.ToolUse{ID: "x", Name: "read_file", Input: []byte(`{"path":"/etc/hosts"}`)}
	res := r.Execute(context.Background(), call)

	if !res.IsError {
		t.Fatal("expected IsError=true for disallowed tool")
	}
}

func TestRegistry_Allowlist_allowsExecution(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})

	r.SetAllowlist([]string{"current_time"})
	call := llm.ToolUse{ID: "y", Name: "current_time", Input: []byte("{}")}
	res := r.Execute(context.Background(), call)

	if res.IsError {
		t.Fatalf("expected success for allowed tool, got: %s", res.Content)
	}
}

func TestRegistry_Allowlist_empty_allowsAll(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(builtin.CurrentTime{})
	r.Register(builtin.ReadFile{})

	// Set then clear the allowlist
	r.SetAllowlist([]string{"current_time"})
	r.SetAllowlist(nil)

	defs := r.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions after clearing allowlist, got %d", len(defs))
	}
}

func TestCurrentTime_schemaIsValidJSON(t *testing.T) {
	def := builtin.CurrentTime{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
