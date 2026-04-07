package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

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

func TestCurrentTime_schemaIsValidJSON(t *testing.T) {
	def := builtin.CurrentTime{}.Definition()
	if !json.Valid(def.InputSchema) {
		t.Errorf("InputSchema is not valid JSON: %s", def.InputSchema)
	}
}
