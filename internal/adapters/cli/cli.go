// Package cli implements the CLI input/output adapter.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
)

// Runner is the subset of agent.Agent used by the CLI adapter.
type Runner interface {
	Run(ctx context.Context, sessionID, input, systemPrompt string) (agent.Result, error)
}

// StreamingRunner is an optional extension of Runner for agents that support
// incremental token streaming.
type StreamingRunner interface {
	Runner
	RunStream(ctx context.Context, sessionID, input, systemPrompt string, handler llm.StreamHandler) (agent.Result, error)
}

// HistoryManager is the subset of agent.History used by REPL commands.
type HistoryManager interface {
	Clear(sessionID string)
	Get(sessionID string) []llm.Message
}

// SkillLoader is called by the REPL when the user types /skill-name.
// It returns the full skill body for the given name, or an error if not found.
type SkillLoader func(name string) (string, error)

// Adapter connects the agent loop to a terminal or piped stdin.
type Adapter struct {
	agent        Runner
	history      HistoryManager // optional; enables /clear and /history commands
	in           io.Reader
	out          io.Writer
	systemPrompt func() string
	verbose      bool
	showUsage    bool
	prefillFn    func() (string, error) // optional; injected into first user message
	skillLoader  SkillLoader            // optional; handles /skill-name REPL commands
	sessionIn    int                    // cumulative input tokens for this session
	sessionOut   int                    // cumulative output tokens for this session
	logger       *slog.Logger
}

// New returns an Adapter wired to the given agent.
// systemPrompt is called on every turn so callers can hot-reload it.
// in/out default to os.Stdin/os.Stdout when nil.
func New(a Runner, systemPrompt func() string, verbose bool, in io.Reader, out io.Writer, logger *slog.Logger) *Adapter {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	return &Adapter{
		agent:        a,
		in:           in,
		out:          out,
		systemPrompt: systemPrompt,
		verbose:      verbose,
		logger:       logger,
	}
}

// SetShowUsage enables per-turn token usage reporting after each response.
func (a *Adapter) SetShowUsage(v bool) {
	a.showUsage = v
}

// SetHistoryManager attaches a HistoryManager that backs the /clear and
// /history REPL commands. Without it those commands print an error.
func (a *Adapter) SetHistoryManager(h HistoryManager) {
	a.history = h
}

// SetPrefill attaches a function that returns a preamble to inject into the
// first user message of a new session. Called once per session start (when
// history is empty). No-op when nil or when it returns an empty string.
func (a *Adapter) SetPrefill(fn func() (string, error)) {
	a.prefillFn = fn
}

// SetSkillLoader attaches a SkillLoader that handles /skill-name REPL
// commands. When set, typing /weather (for example) injects the full skill
// body as the user message and sends it to the agent directly.
func (a *Adapter) SetSkillLoader(fn SkillLoader) {
	a.skillLoader = fn
}

// RunInteractive starts a REPL loop: prints a prompt, reads a line, calls the
// agent, and prints the response. Exits when ctx is cancelled or EOF is reached.
func (a *Adapter) RunInteractive(ctx context.Context, sessionID string) error {
	scanner := bufio.NewScanner(a.in)
	for {
		fmt.Fprint(a.out, "> ")

		// bufio.Scanner doesn't respect context, so check before blocking.
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if !scanner.Scan() {
			// EOF or error.
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("cli: read input: %w", err)
			}
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Check for built-in commands.
		if input == "/exit" || input == "/quit" {
			return nil
		}
		if input == "/clear" {
			if a.history == nil {
				fmt.Fprintln(a.out, "error: history manager not available")
			} else {
				a.history.Clear(sessionID)
				fmt.Fprintln(a.out, "history cleared")
			}
			continue
		}
		if input == "/history" {
			if a.history == nil {
				fmt.Fprintln(a.out, "error: history manager not available")
			} else {
				a.printHistory(sessionID)
			}
			continue
		}

		// /skill-name — user-invoked skill injection.
		// Loads the skill body and sends it as the user message, bypassing
		// autonomous skill selection.
		if strings.HasPrefix(input, "/") {
			skillName := strings.TrimPrefix(input, "/")
			if a.skillLoader != nil {
				body, err := a.skillLoader(skillName)
				if err != nil {
					fmt.Fprintf(a.out, "error: %v\n", err)
					continue
				}
				input = body
			} else {
				fmt.Fprintf(a.out, "error: unknown command %q\n", input)
				continue
			}
		}

		a.logger.Debug("user input", "session_id", sessionID, "input", input)

		result, streamed, err := a.runTurn(ctx, sessionID, input)
		if err != nil {
			fmt.Fprintf(a.out, "error: %v\n", err)
			continue
		}

		if a.verbose {
			a.printVerbose(result)
		}
		if streamed {
			fmt.Fprintln(a.out) // trailing newline after streamed output
		} else {
			fmt.Fprintln(a.out, result.Text)
		}
		if a.showUsage {
			a.printUsage(result)
		}
	}
}

// RunOnce sends a single input to the agent and writes the response to out.
// Suitable for non-interactive / piped usage.
func (a *Adapter) RunOnce(ctx context.Context, sessionID, input string) error {
	input, err := a.maybeInjectPrefill(sessionID, input)
	if err != nil {
		return fmt.Errorf("cli: build prefill: %w", err)
	}
	result, err := a.agent.Run(ctx, sessionID, input, a.systemPrompt())
	if err != nil {
		return fmt.Errorf("cli: agent run: %w", err)
	}
	if a.verbose {
		a.printVerbose(result)
	}
	fmt.Fprintln(a.out, result.Text)
	if a.showUsage {
		a.printUsage(result)
	}
	return nil
}

// runTurn dispatches one agent call, using streaming when available.
// streamed reports whether any text was delivered via the stream handler.
func (a *Adapter) runTurn(ctx context.Context, sessionID, input string) (result agent.Result, streamed bool, err error) {
	input, err = a.maybeInjectPrefill(sessionID, input)
	if err != nil {
		return result, false, fmt.Errorf("cli: build prefill: %w", err)
	}
	if sr, ok := a.agent.(StreamingRunner); ok {
		result, err = sr.RunStream(ctx, sessionID, input, a.systemPrompt(), func(delta string) {
			streamed = true
			fmt.Fprint(a.out, delta)
		})
		return result, streamed, err
	}
	result, err = a.agent.Run(ctx, sessionID, input, a.systemPrompt())
	return result, false, err
}

// maybeInjectPrefill prepends the prefill preamble to input when a prefillFn
// is set and the session has no prior history. Returns input unchanged otherwise.
func (a *Adapter) maybeInjectPrefill(sessionID, input string) (string, error) {
	if a.prefillFn == nil {
		return input, nil
	}
	if a.history != nil && len(a.history.Get(sessionID)) > 0 {
		return input, nil // not first message
	}
	preamble, err := a.prefillFn()
	if err != nil {
		return "", err
	}
	if preamble == "" {
		return input, nil
	}
	return preamble + "\n" + input, nil
}

// printHistory writes a human-readable summary of the session's message
// history to out. Tool-result messages (role=tool) are omitted; tool calls
// inside assistant messages are shown as [tool: name].
func (a *Adapter) printHistory(sessionID string) {
	msgs := a.history.Get(sessionID)
	if len(msgs) == 0 {
		fmt.Fprintln(a.out, "(no history)")
		return
	}
	for i, m := range msgs {
		switch m.Role {
		case llm.RoleTool:
			// internal tool-result turn — skip
		case llm.RoleUser:
			text := m.TextContent()
			if text != "" {
				fmt.Fprintf(a.out, "[%d] you: %s\n", i+1, text)
			}
		case llm.RoleAssistant:
			text := m.TextContent()
			if text != "" {
				fmt.Fprintf(a.out, "[%d] assistant: %s\n", i+1, text)
			}
			for _, tu := range m.ToolUses() {
				fmt.Fprintf(a.out, "[%d] assistant: [tool: %s]\n", i+1, tu.Name)
			}
		}
	}
}

// printVerbose writes thinking blocks and tool calls to out before the response.
func (a *Adapter) printVerbose(r agent.Result) {
	for i, t := range r.Thinking {
		fmt.Fprintf(a.out, "[thinking %d]\n%s\n\n", i+1, t)
	}
	for _, tc := range r.ToolCalls {
		fmt.Fprintf(a.out, "[tool: %s]\n", tc.Name)
		if len(tc.Input) > 0 {
			fmt.Fprintf(a.out, "input: %s\n", tc.Input)
		}
		if tc.IsError {
			fmt.Fprintf(a.out, "error: %s\n", tc.Result)
		} else {
			fmt.Fprintf(a.out, "result: %s\n", tc.Result)
		}
		fmt.Fprintln(a.out)
	}
}

// printUsage writes per-turn and cumulative session token counts to out.
func (a *Adapter) printUsage(r agent.Result) {
	a.sessionIn += r.Usage.InputTokens
	a.sessionOut += r.Usage.OutputTokens
	fmt.Fprintf(a.out,
		"[usage] turn: in=%d out=%d | session: in=%d out=%d\n",
		r.Usage.InputTokens, r.Usage.OutputTokens,
		a.sessionIn, a.sessionOut,
	)
}

// RunStdin reads all of stdin as a single input and calls RunOnce.
func (a *Adapter) RunStdin(ctx context.Context, sessionID string) error {
	data, err := io.ReadAll(a.in)
	if err != nil {
		return fmt.Errorf("cli: read stdin: %w", err)
	}
	input := strings.TrimSpace(string(data))
	if input == "" {
		return fmt.Errorf("cli: empty input")
	}
	return a.RunOnce(ctx, sessionID, input)
}

// IsTerminal reports whether r is an interactive terminal.
// Used by callers to decide between RunInteractive and RunStdin.
func IsTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// compile-time checks.
var _ Runner = (*agent.Agent)(nil)
var _ StreamingRunner = (*agent.Agent)(nil)
