package slashcmd

import (
	"context"
	"fmt"
	"strings"
)

// RegisterBuiltins adds the standard built-in commands to r.
// These are channel-agnostic; all adapters should register them.
//
//   - /help    — list all registered commands
//   - /clear   — clear conversation history for the current session
//   - /history — print conversation history for the current session
//   - /exit    — signal the adapter to quit (CLI REPL only; no-op on Telegram)
func RegisterBuiltins(r *Registry) {
	r.Register(Command{
		Name:        "help",
		Description: "List available commands",
		Handler:     helpHandler(r),
	})
	r.Register(Command{
		Name:        "clear",
		Description: "Clear conversation history for this session",
		Handler:     clearHandler,
	})
	r.Register(Command{
		Name:        "history",
		Description: "Print conversation history for this session",
		Handler:     historyHandler,
	})
	r.Register(Command{
		Name:        "exit",
		Description: "Exit the REPL (CLI only)",
		Handler:     exitHandler,
	})
}

func helpHandler(r *Registry) HandlerFunc {
	return func(_ context.Context, _ string, _ Env) Response {
		var sb strings.Builder
		sb.WriteString("Available commands:\n")
		for _, cmd := range r.All() {
			if cmd.Args != "" {
				fmt.Fprintf(&sb, "  /%s %s — %s\n", cmd.Name, cmd.Args, cmd.Description)
			} else {
				fmt.Fprintf(&sb, "  /%s — %s\n", cmd.Name, cmd.Description)
			}
		}
		return Response{Text: strings.TrimRight(sb.String(), "\n")}
	}
}

func clearHandler(_ context.Context, _ string, env Env) Response {
	if env.History == nil {
		return Response{Text: "error: history not available"}
	}
	env.History.Clear(env.SessionID)
	return Response{Text: "conversation history cleared"}
}

func historyHandler(_ context.Context, _ string, env Env) Response {
	if env.History == nil {
		return Response{Text: "error: history not available"}
	}
	lines := env.History.Lines(env.SessionID)
	if len(lines) == 0 {
		return Response{Text: "(no history)"}
	}
	return Response{Text: strings.Join(lines, "\n")}
}

func exitHandler(_ context.Context, _ string, _ Env) Response {
	return Response{Text: "bye", Action: ActionExit}
}
