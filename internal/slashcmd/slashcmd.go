// Package slashcmd provides a channel-agnostic slash command registry and
// dispatcher. Slash commands are intercepted before input reaches the agent
// loop — no LLM call is involved in handling them.
//
// Adapters (CLI, Telegram, etc.) are responsible for detecting the "/" prefix
// and calling Dispatch. The registry defines and executes commands; adapters
// handle presentation of the response.
package slashcmd

import (
	"context"
	"fmt"
	"strings"
)

// Action signals special adapter behaviour beyond returning text.
type Action int

const (
	// ActionNone means just return Text to the user.
	ActionNone Action = iota
	// ActionExit signals the adapter to terminate its session/REPL.
	ActionExit
)

// Response is returned by a command handler.
type Response struct {
	Text   string
	Action Action
}

// Env carries per-invocation context that commands may need.
// Fields are optional; commands must nil-check before use.
type Env struct {
	SessionID string
	History   HistoryManager
}

// HistoryManager is the subset of the history API that slash commands use.
type HistoryManager interface {
	Clear(sessionID string)
	Lines(sessionID string) []string // human-readable lines for /history
}

// HandlerFunc is the signature for a command handler.
// args is everything after the command name (trimmed), may be empty.
type HandlerFunc func(ctx context.Context, args string, env Env) Response

// Command is a single slash command entry.
type Command struct {
	Name        string // without leading slash, e.g. "clear"
	Description string // shown in /help and Telegram BotFather
	Args        string // optional usage hint, e.g. "" or "<name>"
	Handler     HandlerFunc
}

// Registry holds registered commands and dispatches invocations.
type Registry struct {
	cmds []Command
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{}
}

// Register adds a command. Panics if name is empty or already registered.
func (r *Registry) Register(cmd Command) {
	if cmd.Name == "" {
		panic("slashcmd: command name must not be empty")
	}
	for _, c := range r.cmds {
		if c.Name == cmd.Name {
			panic(fmt.Sprintf("slashcmd: duplicate command %q", cmd.Name))
		}
	}
	r.cmds = append(r.cmds, cmd)
}

// All returns all registered commands in registration order.
func (r *Registry) All() []Command {
	out := make([]Command, len(r.cmds))
	copy(out, r.cmds)
	return out
}

// Dispatch parses input and calls the matching command handler.
// input must start with "/"; returns (response, true) on match,
// (zero, false) if no command matches.
// Unknown slash commands return a helpful error response with (_, true).
func (r *Registry) Dispatch(ctx context.Context, input string, env Env) (Response, bool) {
	if !strings.HasPrefix(input, "/") {
		return Response{}, false
	}
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), " ", 2)
	name := parts[0]
	args := ""
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}

	for _, cmd := range r.cmds {
		if cmd.Name == name {
			return cmd.Handler(ctx, args, env), true
		}
	}

	// Unknown command — return error without touching agent.
	return Response{
		Text: fmt.Sprintf("unknown command /%s — type /help for available commands", name),
	}, true
}
