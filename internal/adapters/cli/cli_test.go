package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
	"log/slog"
)

// stubRunner always returns a fixed response.
type stubRunner struct{ text string }

func (s stubRunner) Run(_ context.Context, _, _, _ string) (agent.Result, error) {
	return agent.Result{Text: s.text}, nil
}

// stubHistory is a minimal in-memory HistoryManager for tests.
type stubHistory struct {
	msgs map[string][]llm.Message
}

func newStubHistory() *stubHistory {
	return &stubHistory{msgs: make(map[string][]llm.Message)}
}

func (h *stubHistory) Clear(sessionID string) { delete(h.msgs, sessionID) }

func (h *stubHistory) Get(sessionID string) []llm.Message { return h.msgs[sessionID] }

func (h *stubHistory) add(sessionID string, msgs ...llm.Message) {
	h.msgs[sessionID] = append(h.msgs[sessionID], msgs...)
}

func newAdapter(input string, hist cli.HistoryManager) (*cli.Adapter, *bytes.Buffer) {
	in := strings.NewReader(input)
	out := &bytes.Buffer{}
	a := cli.New(stubRunner{"ok"}, func() string { return "" }, false, in, out, slog.Default())
	if hist != nil {
		a.SetHistoryManager(hist)
	}
	return a, out
}

func TestREPL_clear_withHistory(t *testing.T) {
	hist := newStubHistory()
	hist.add("s1",
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hello"}}},
	)

	a, out := newAdapter("/clear\n/exit\n", hist)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}

	if got := hist.Get("s1"); len(got) != 0 {
		t.Errorf("expected empty history after /clear, got %d messages", len(got))
	}
	if !strings.Contains(out.String(), "history cleared") {
		t.Errorf("expected confirmation message, got: %q", out.String())
	}
}

func TestREPL_clear_withoutHistory(t *testing.T) {
	a, out := newAdapter("/clear\n/exit\n", nil)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "error:") {
		t.Errorf("expected error message when no history manager, got: %q", out.String())
	}
}

func TestREPL_history_empty(t *testing.T) {
	hist := newStubHistory()
	a, out := newAdapter("/history\n/exit\n", hist)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no history") {
		t.Errorf("expected 'no history' message, got: %q", out.String())
	}
}

func TestREPL_history_showsMessages(t *testing.T) {
	hist := newStubHistory()
	hist.add("s1",
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "hi"}}},
		llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Text: "hello there"}}},
	)

	a, out := newAdapter("/history\n/exit\n", hist)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "you: hi") {
		t.Errorf("expected user message in history output, got: %q", got)
	}
	if !strings.Contains(got, "assistant: hello there") {
		t.Errorf("expected assistant message in history output, got: %q", got)
	}
}

func TestREPL_history_skipsToolResults(t *testing.T) {
	hist := newStubHistory()
	hist.add("s1",
		llm.Message{Role: llm.RoleTool, Content: []llm.ContentBlock{
			{ToolResult: &llm.ToolResult{ToolUseID: "x", Content: "internal"}},
		}},
		llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "visible"}}},
	)

	a, out := newAdapter("/history\n/exit\n", hist)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if strings.Contains(got, "internal") {
		t.Errorf("tool-result content should not appear in /history output, got: %q", got)
	}
	if !strings.Contains(got, "you: visible") {
		t.Errorf("expected user message to appear, got: %q", got)
	}
}

func TestREPL_history_withoutHistoryManager(t *testing.T) {
	a, out := newAdapter("/history\n/exit\n", nil)
	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "error:") {
		t.Errorf("expected error message when no history manager, got: %q", out.String())
	}
}
