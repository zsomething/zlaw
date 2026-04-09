package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chickenzord/zlaw/internal/llm"
)

const maxIterations = 20

// ToolExecutor is the subset of tools.Registry used by the agent loop.
type ToolExecutor interface {
	Definitions() []llm.ToolDefinition
	ExecuteAll(ctx context.Context, calls []llm.ToolUse) []llm.ToolResult
}

// ToolCall records a single tool invocation and its result for verbose output.
type ToolCall struct {
	Name    string
	Input   []byte
	Result  string
	IsError bool
}

// Result holds the output of one agent turn.
type Result struct {
	Text      string
	Thinking  []string // one entry per LLM iteration that produced thinking
	ToolCalls []ToolCall
	Usage     llm.Usage // cumulative token usage across all LLM calls in this turn
}

// Agent runs the ReAct loop for a single agent instance.
type Agent struct {
	name      string
	client    llm.Client
	tools     ToolExecutor
	history   *History
	logger    *slog.Logger
	optimizer *ContextOptimizer // nil = no context optimization
}

// New creates an Agent. All parameters are required.
func New(name string, client llm.Client, tools ToolExecutor, history *History, logger *slog.Logger) *Agent {
	return &Agent{
		name:    name,
		client:  client,
		tools:   tools,
		history: history,
		logger:  logger,
	}
}

// SetContextOptimizer attaches a ContextOptimizer that runs the
// summarize→prune pipeline before each LLM call. Pass nil to disable.
func (a *Agent) SetContextOptimizer(o *ContextOptimizer) {
	a.optimizer = o
}

// SetContextTokenBudget is a convenience method that configures prune-only
// context management (no summarization). Use SetContextOptimizer for the full
// summarize→prune pipeline.
func (a *Agent) SetContextTokenBudget(budget int) {
	a.optimizer = NewContextOptimizer(
		ContextOptimizerConfig{TokenBudget: budget},
		nil,   // no summarizer
		a.logger,
	)
}

// Run executes one user turn for the given session.
//
// It appends the user input to history, then drives the ReAct loop until the
// model emits a final text response, an error occurs, or maxIterations is
// reached. The final text response is returned and also appended to history.
func (a *Agent) Run(ctx context.Context, sessionID, input, systemPrompt string) (Result, error) {
	return a.run(ctx, sessionID, input, systemPrompt, nil)
}

// RunStream is like Run but calls handler with each text delta as tokens
// arrive, when the underlying LLM client supports streaming. If streaming is
// not supported, it falls back to a non-streaming call without error.
func (a *Agent) RunStream(ctx context.Context, sessionID, input, systemPrompt string, handler llm.StreamHandler) (Result, error) {
	return a.run(ctx, sessionID, input, systemPrompt, handler)
}

func (a *Agent) run(ctx context.Context, sessionID, input, systemPrompt string, handler llm.StreamHandler) (Result, error) {
	log := a.logger.With("agent", a.name, "session_id", sessionID)

	sc, hasStreaming := a.client.(llm.StreamingClient)

	a.history.Append(sessionID, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Text: input}},
	})

	var result Result

	for i := range maxIterations {
		log.Debug("llm call", "iteration", i+1)

		allMsgs := a.history.Get(sessionID)
		msgs := allMsgs
		if a.optimizer != nil {
			msgs = a.optimizer.Optimize(ctx, allMsgs)
		}

		req := llm.Request{
			SystemPrompt: systemPrompt,
			Messages:     msgs,
			Tools:        a.tools.Definitions(),
		}

		var (
			resp llm.Response
			err  error
		)
		if handler != nil && hasStreaming {
			resp, err = sc.CompleteStream(ctx, req, handler)
		} else {
			resp, err = a.client.Complete(ctx, req)
		}
		if err != nil {
			return Result{}, fmt.Errorf("agent: llm call: %w", err)
		}

		result.Usage.InputTokens += resp.Usage.InputTokens
		result.Usage.OutputTokens += resp.Usage.OutputTokens

		log.Debug("llm response", "stop_reason", resp.StopReason,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens)

		if t := resp.Message.ThinkingContent(); t != "" {
			result.Thinking = append(result.Thinking, t)
		}

		a.history.Append(sessionID, resp.Message)

		switch resp.StopReason {
		case "end_turn":
			result.Text = resp.Message.TextContent()
			a.history.RecordUsage(sessionID, result.Usage)
			return result, nil

		case "tool_use":
			calls := resp.Message.ToolUses()
			log.Debug("executing tools", "count", len(calls))

			results := a.tools.ExecuteAll(ctx, calls)

			blocks := make([]llm.ContentBlock, len(results))
			for j, r := range results {
				r := r // capture
				blocks[j] = llm.ContentBlock{ToolResult: &r}
				tc := ToolCall{Result: r.Content, IsError: r.IsError}
				if j < len(calls) {
					tc.Name = calls[j].Name
					tc.Input = calls[j].Input
				}
				result.ToolCalls = append(result.ToolCalls, tc)
				if r.IsError {
					log.Warn("tool error", "tool_use_id", r.ToolUseID, "error", r.Content)
				} else {
					log.Debug("tool result", "tool_use_id", r.ToolUseID)
				}
			}
			a.history.Append(sessionID, llm.Message{
				Role:    llm.RoleTool,
				Content: blocks,
			})

		case "max_tokens":
			return Result{}, fmt.Errorf("agent: response truncated (max_tokens reached)")

		default:
			return Result{}, fmt.Errorf("agent: unexpected stop_reason %q", resp.StopReason)
		}
	}

	return Result{}, fmt.Errorf("agent: exceeded max iterations (%d)", maxIterations)
}
