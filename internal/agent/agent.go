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
}

// Agent runs the ReAct loop for a single agent instance.
type Agent struct {
	name               string
	client             llm.Client
	tools              ToolExecutor
	history            *History
	logger             *slog.Logger
	contextTokenBudget int // 0 = no pruning
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

// SetContextTokenBudget configures the maximum estimated token count for the
// message history sent to the LLM. Oldest turns are pruned when exceeded.
// Zero (the default) disables pruning.
func (a *Agent) SetContextTokenBudget(budget int) {
	a.contextTokenBudget = budget
}

// Run executes one user turn for the given session.
//
// It appends the user input to history, then drives the ReAct loop until the
// model emits a final text response, an error occurs, or maxIterations is
// reached. The final text response is returned and also appended to history.
func (a *Agent) Run(ctx context.Context, sessionID, input, systemPrompt string) (Result, error) {
	log := a.logger.With("agent", a.name, "session_id", sessionID)

	a.history.Append(sessionID, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Text: input}},
	})

	var result Result

	for i := range maxIterations {
		log.Debug("llm call", "iteration", i+1)

		allMsgs := a.history.Get(sessionID)
		msgs := PruneMessages(allMsgs, a.contextTokenBudget)
		if len(msgs) < len(allMsgs) {
			log.Debug("context pruned", "full_messages", len(allMsgs), "pruned_messages", len(msgs))
		}

		req := llm.Request{
			SystemPrompt: systemPrompt,
			Messages:     msgs,
			Tools:        a.tools.Definitions(),
		}

		resp, err := a.client.Complete(ctx, req)
		if err != nil {
			return Result{}, fmt.Errorf("agent: llm call: %w", err)
		}

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
