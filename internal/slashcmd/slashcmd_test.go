package slashcmd_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/slashcmd"
)

// --- stub helpers ---

type stubHistory struct {
	cleared []string
	lines   map[string][]string
}

func (h *stubHistory) Clear(sessionID string)       { h.cleared = append(h.cleared, sessionID) }
func (h *stubHistory) Lines(sessionID string) []string { return h.lines[sessionID] }

func newRegistry() *slashcmd.Registry {
	r := slashcmd.New()
	slashcmd.RegisterBuiltins(r)
	return r
}

// --- Registry.Dispatch tests ---

func TestDispatch_NoPrefixReturnsFalse(t *testing.T) {
	r := slashcmd.New()
	_, matched := r.Dispatch(context.Background(), "hello world", slashcmd.Env{})
	if matched {
		t.Fatal("input without '/' should not match")
	}
}

func TestDispatch_EmptyInputReturnsFalse(t *testing.T) {
	r := slashcmd.New()
	_, matched := r.Dispatch(context.Background(), "", slashcmd.Env{})
	if matched {
		t.Fatal("empty input should not match")
	}
}

func TestDispatch_KnownCommandInvokesHandler(t *testing.T) {
	r := slashcmd.New()
	called := false
	r.Register(slashcmd.Command{
		Name:    "ping",
		Handler: func(_ context.Context, _ string, _ slashcmd.Env) slashcmd.Response {
			called = true
			return slashcmd.Response{Text: "pong"}
		},
	})

	resp, matched := r.Dispatch(context.Background(), "/ping", slashcmd.Env{})
	if !matched {
		t.Fatal("expected match for /ping")
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if resp.Text != "pong" {
		t.Fatalf("expected 'pong', got %q", resp.Text)
	}
}

func TestDispatch_UnknownCommandReturnsHelpfulError(t *testing.T) {
	r := slashcmd.New()

	resp, matched := r.Dispatch(context.Background(), "/unknown", slashcmd.Env{})
	if !matched {
		t.Fatal("unknown command should still return matched=true")
	}
	if !strings.Contains(resp.Text, "unknown") {
		t.Fatalf("error message should mention the command name, got %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "/help") {
		t.Fatalf("error message should suggest /help, got %q", resp.Text)
	}
}

func TestDispatch_ArgsArePassedToHandler(t *testing.T) {
	r := slashcmd.New()
	var gotArgs string
	r.Register(slashcmd.Command{
		Name: "echo",
		Handler: func(_ context.Context, args string, _ slashcmd.Env) slashcmd.Response {
			gotArgs = args
			return slashcmd.Response{Text: args}
		},
	})

	r.Dispatch(context.Background(), "/echo  hello world  ", slashcmd.Env{})
	if gotArgs != "hello world" {
		t.Fatalf("expected trimmed args 'hello world', got %q", gotArgs)
	}
}

func TestDispatch_ArgsEmptyWhenNoArgs(t *testing.T) {
	r := slashcmd.New()
	var gotArgs string
	r.Register(slashcmd.Command{
		Name: "noop",
		Handler: func(_ context.Context, args string, _ slashcmd.Env) slashcmd.Response {
			gotArgs = args
			return slashcmd.Response{}
		},
	})

	r.Dispatch(context.Background(), "/noop", slashcmd.Env{})
	if gotArgs != "" {
		t.Fatalf("expected empty args, got %q", gotArgs)
	}
}

// --- Registry.Register tests ---

func TestRegister_EmptyNamePanics(t *testing.T) {
	r := slashcmd.New()
	defer func() {
		if recover() == nil {
			t.Error("expected panic for empty name")
		}
	}()
	r.Register(slashcmd.Command{Name: ""})
}

func TestRegister_DuplicateNamePanics(t *testing.T) {
	r := slashcmd.New()
	r.Register(slashcmd.Command{Name: "foo", Handler: func(_ context.Context, _ string, _ slashcmd.Env) slashcmd.Response { return slashcmd.Response{} }})
	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate name")
		}
	}()
	r.Register(slashcmd.Command{Name: "foo", Handler: func(_ context.Context, _ string, _ slashcmd.Env) slashcmd.Response { return slashcmd.Response{} }})
}

func TestAll_ReturnsCopy(t *testing.T) {
	r := slashcmd.New()
	r.Register(slashcmd.Command{Name: "a", Handler: func(_ context.Context, _ string, _ slashcmd.Env) slashcmd.Response { return slashcmd.Response{} }})

	cmds := r.All()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	// Mutating the returned slice should not affect the registry.
	cmds[0].Name = "mutated"
	if r.All()[0].Name != "a" {
		t.Fatal("All() should return a copy — mutation leaked into registry")
	}
}

// --- Built-in: /help ---

func TestHelp_ListsRegisteredCommands(t *testing.T) {
	r := newRegistry()
	resp, matched := r.Dispatch(context.Background(), "/help", slashcmd.Env{})

	if !matched {
		t.Fatal("expected /help to match")
	}
	for _, name := range []string{"help", "clear", "history", "exit"} {
		if !strings.Contains(resp.Text, "/"+name) {
			t.Errorf("/help output should mention /%s, got:\n%s", name, resp.Text)
		}
	}
}

func TestHelp_IncludesArgsHint(t *testing.T) {
	r := slashcmd.New()
	r.Register(slashcmd.Command{
		Name:        "model",
		Args:        "<name>",
		Description: "switch model",
		Handler:     func(_ context.Context, _ string, _ slashcmd.Env) slashcmd.Response { return slashcmd.Response{} },
	})
	slashcmd.RegisterBuiltins(r)

	resp, _ := r.Dispatch(context.Background(), "/help", slashcmd.Env{})
	if !strings.Contains(resp.Text, "<name>") {
		t.Errorf("/help should include Args hint for commands that have one, got:\n%s", resp.Text)
	}
}

// --- Built-in: /clear ---

func TestClear_NoHistory_ReturnsError(t *testing.T) {
	r := newRegistry()
	resp, matched := r.Dispatch(context.Background(), "/clear", slashcmd.Env{})

	if !matched {
		t.Fatal("expected /clear to match")
	}
	if !strings.Contains(resp.Text, "error") {
		t.Fatalf("/clear without history should return error message, got %q", resp.Text)
	}
}

func TestClear_ClearsHistoryForSession(t *testing.T) {
	r := newRegistry()
	h := &stubHistory{}
	env := slashcmd.Env{SessionID: "s1", History: h}

	resp, _ := r.Dispatch(context.Background(), "/clear", env)
	if resp.Text != "conversation history cleared" {
		t.Fatalf("unexpected response: %q", resp.Text)
	}
	if len(h.cleared) != 1 || h.cleared[0] != "s1" {
		t.Fatalf("expected Clear called with 's1', got: %v", h.cleared)
	}
}

func TestClear_FiresAfterClearCallback(t *testing.T) {
	r := newRegistry()
	h := &stubHistory{}
	var callbackArg string
	env := slashcmd.Env{
		SessionID: "s2",
		History:   h,
		AfterClear: func(old string) { callbackArg = old },
	}

	r.Dispatch(context.Background(), "/clear", env)
	if callbackArg != "s2" {
		t.Fatalf("AfterClear should be called with old session ID 's2', got %q", callbackArg)
	}
}

func TestClear_NoAfterClear_NoPanic(t *testing.T) {
	r := newRegistry()
	h := &stubHistory{}
	env := slashcmd.Env{SessionID: "s3", History: h} // AfterClear is nil

	// Must not panic.
	r.Dispatch(context.Background(), "/clear", env)
}

// --- Built-in: /history ---

func TestHistory_NoHistory_ReturnsError(t *testing.T) {
	r := newRegistry()
	resp, matched := r.Dispatch(context.Background(), "/history", slashcmd.Env{})

	if !matched {
		t.Fatal("expected /history to match")
	}
	if !strings.Contains(resp.Text, "error") {
		t.Fatalf("/history without history should return error message, got %q", resp.Text)
	}
}

func TestHistory_EmptySession_ReturnsNoHistoryMsg(t *testing.T) {
	r := newRegistry()
	h := &stubHistory{lines: map[string][]string{"s1": {}}}
	env := slashcmd.Env{SessionID: "s1", History: h}

	resp, _ := r.Dispatch(context.Background(), "/history", env)
	if resp.Text != "(no history)" {
		t.Fatalf("expected '(no history)', got %q", resp.Text)
	}
}

func TestHistory_WithLines_JoinsLines(t *testing.T) {
	r := newRegistry()
	h := &stubHistory{lines: map[string][]string{
		"s1": {"user: hello", "assistant: hi"},
	}}
	env := slashcmd.Env{SessionID: "s1", History: h}

	resp, _ := r.Dispatch(context.Background(), "/history", env)
	if !strings.Contains(resp.Text, "user: hello") {
		t.Errorf("expected history lines in response, got %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "assistant: hi") {
		t.Errorf("expected history lines in response, got %q", resp.Text)
	}
}

// --- Built-in: /exit ---

func TestExit_ReturnsActionExit(t *testing.T) {
	r := newRegistry()
	resp, matched := r.Dispatch(context.Background(), "/exit", slashcmd.Env{})

	if !matched {
		t.Fatal("expected /exit to match")
	}
	if resp.Action != slashcmd.ActionExit {
		t.Fatalf("expected ActionExit, got %v", resp.Action)
	}
}
