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

// Agent runs the ReAct loop for a single agent instance.
type Agent struct {
	name    string
	client  llm.Client
	tools   ToolExecutor
	history *History
	logger  *slog.Logger
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

// Run executes one user turn for the given session.
//
// It appends the user input to history, then drives the ReAct loop until the
// model emits a final text response, an error occurs, or maxIterations is
// reached. The final text response is returned and also appended to history.
func (a *Agent) Run(ctx context.Context, sessionID, input, systemPrompt string) (string, error) {
	log := a.logger.With("agent", a.name, "session_id", sessionID)

	a.history.Append(sessionID, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Text: input}},
	})

	for i := range maxIterations {
		log.Debug("llm call", "iteration", i+1)

		req := llm.Request{
			SystemPrompt: systemPrompt,
			Messages:     a.history.Get(sessionID),
			Tools:        a.tools.Definitions(),
		}

		resp, err := a.client.Complete(ctx, req)
		if err != nil {
			return "", fmt.Errorf("agent: llm call: %w", err)
		}

		log.Debug("llm response", "stop_reason", resp.StopReason,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens)

		a.history.Append(sessionID, resp.Message)

		switch resp.StopReason {
		case "end_turn":
			return resp.Message.TextContent(), nil

		case "tool_use":
			calls := resp.Message.ToolUses()
			log.Debug("executing tools", "count", len(calls))

			results := a.tools.ExecuteAll(ctx, calls)

			blocks := make([]llm.ContentBlock, len(results))
			for j, r := range results {
				r := r // capture
				blocks[j] = llm.ContentBlock{ToolResult: &r}
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
			return "", fmt.Errorf("agent: response truncated (max_tokens reached)")

		default:
			return "", fmt.Errorf("agent: unexpected stop_reason %q", resp.StopReason)
		}
	}

	return "", fmt.Errorf("agent: exceeded max iterations (%d)", maxIterations)
}
