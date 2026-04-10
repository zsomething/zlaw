package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"log/slog"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/slashcmd"
)

// stubRunner always returns a fixed response.
type stubRunner struct{ text string }

func (s stubRunner) Run(_ context.Context, _, _, _ string) (agent.Result, error) {
	return agent.Result{Text: s.text}, nil
}

// captureRunner captures the input passed to the last Run call.
type captureRunner struct {
	lastInput string
}

func (c *captureRunner) Run(_ context.Context, _, input, _ string) (agent.Result, error) {
	c.lastInput = input
	return agent.Result{Text: "ok"}, nil
}

// stubHistory is a minimal in-memory HistoryManager for tests.
type stubHistory struct {
	msgs map[string][]llm.Message
}

func newStubHistory() *stubHistory {
	return &stubHistory{msgs: make(map[string][]llm.Message)}
}

func (h *stubHistory) Clear(sessionID string) { delete(h.msgs, sessionID) }

// Get is a test helper — not part of slashcmd.HistoryManager.
func (h *stubHistory) Get(sessionID string) []llm.Message { return h.msgs[sessionID] }

func (h *stubHistory) Lines(sessionID string) []string {
	msgs := h.msgs[sessionID]
	var lines []string
	for i, m := range msgs {
		switch m.Role {
		case llm.RoleTool:
			// skip
		case llm.RoleUser:
			if text := m.TextContent(); text != "" {
				lines = append(lines, fmt.Sprintf("[%d] you: %s", i+1, text))
			}
		case llm.RoleAssistant:
			if text := m.TextContent(); text != "" {
				lines = append(lines, fmt.Sprintf("[%d] assistant: %s", i+1, text))
			}
		}
	}
	return lines
}

func (h *stubHistory) add(sessionID string, msgs ...llm.Message) {
	h.msgs[sessionID] = append(h.msgs[sessionID], msgs...)
}

func newAdapter(input string, hist slashcmd.HistoryManager) (*cli.Adapter, *bytes.Buffer) {
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

func TestPrefill_InjectedOnFirstTurn(t *testing.T) {
	runner := &captureRunner{}
	hist := newStubHistory()
	in := strings.NewReader("hello\n/exit\n")
	out := &bytes.Buffer{}
	a := cli.New(runner, func() string { return "" }, false, in, out, slog.Default())
	a.SetHistoryManager(hist)
	a.SetPrefill(func() (string, error) { return "[Session context]\ncwd: /test\n", nil })

	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runner.lastInput, "[Session context]") {
		t.Errorf("expected prefill prepended on first turn, got: %q", runner.lastInput)
	}
	if !strings.Contains(runner.lastInput, "hello") {
		t.Errorf("expected user input in message, got: %q", runner.lastInput)
	}
}

func TestPrefill_NotInjectedWhenHistoryExists(t *testing.T) {
	runner := &captureRunner{}
	hist := newStubHistory()
	// Pre-seed history so this is not the first message.
	hist.add("s1", llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Text: "prior"}}})

	in := strings.NewReader("hello\n/exit\n")
	out := &bytes.Buffer{}
	a := cli.New(runner, func() string { return "" }, false, in, out, slog.Default())
	a.SetHistoryManager(hist)
	a.SetPrefill(func() (string, error) { return "[Session context]\ncwd: /test\n", nil })

	if err := a.RunInteractive(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(runner.lastInput, "[Session context]") {
		t.Errorf("prefill should not be injected when history is non-empty, got: %q", runner.lastInput)
	}
}
