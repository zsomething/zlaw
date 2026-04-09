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

// Adapter connects the agent loop to a terminal or piped stdin.
type Adapter struct {
	agent        Runner
	in           io.Reader
	out          io.Writer
	systemPrompt func() string
	verbose      bool
	showUsage    bool
	sessionIn    int // cumulative input tokens for this session
	sessionOut   int // cumulative output tokens for this session
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
