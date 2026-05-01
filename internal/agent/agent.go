package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/zsomething/zlaw/internal/ctxkey"
	"github.com/zsomething/zlaw/internal/llm"
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
	id              string
	client          llm.Client
	tools           ToolExecutor
	history         *History
	logger          *slog.Logger
	optimizer       *ContextOptimizer // nil = no context optimization
	stickyBlocks    []StickyBlock
	skillsSection   string      // pre-built [Available Skills] block; "" = no skills
	memoryStore     MemoryStore // nil = memories disabled
	maxMemoryTokens int
}

// New creates an Agent. All parameters are required.
// id is the stable agent identifier.
func New(id string, client llm.Client, tools ToolExecutor, history *History, logger *slog.Logger) *Agent {
	return &Agent{
		id:      id,
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

// SetStickyBlocks configures framework-level instruction blocks prepended to
// every system prompt as the stable head (cache checkpoint 1). Blocks are
// ordered; sticky content lives in Go source, not agent config files.
func (a *Agent) SetStickyBlocks(blocks []StickyBlock) {
	a.stickyBlocks = blocks
}

// SetSkillsSection sets the pre-built [Available Skills] index block to inject
// as a stable cached section between personality and memories. Pass an empty
// string to disable. Use BuildSkillsSection to build the block.
func (a *Agent) SetSkillsSection(section string) {
	a.skillsSection = section
}

// SetMemoryStore attaches a MemoryStore whose contents are injected as a
// [Memories] block at the end of each LLM request's system prompt (uncached,
// intentionally volatile). maxTokens caps the block size; 0 means unlimited.
func (a *Agent) SetMemoryStore(store MemoryStore, maxTokens int) {
	a.memoryStore = store
	a.maxMemoryTokens = maxTokens
}

// SetContextTokenBudget is a convenience method that configures prune-only
// context management (no summarization). Use SetContextOptimizer for the full
// summarize→prune pipeline.
func (a *Agent) SetContextTokenBudget(budget int) {
	a.optimizer = NewContextOptimizer(
		ContextOptimizerConfig{TokenBudget: budget},
		nil, // no summarizer
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
	ctx = context.WithValue(ctx, ctxkey.SessionID, sessionID)
	log := a.logger.With("agent", a.id, "session_id", sessionID)

	log.Debug("turn started", "input_len", len(input), "system_prompt_len", len(systemPrompt))
	log.Debug("system prompt", "system_prompt", systemPrompt)

	sc, hasStreaming := a.client.(llm.StreamingClient)

	a.history.Append(sessionID, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{{Text: input}},
	})

	var result Result
	var lastInputTokens int // seeded from resp.Usage after each call

	for i := range maxIterations {
		log.Debug("llm call", "iteration", i+1)

		allMsgs := a.history.Get(sessionID)
		msgs := allMsgs
		if a.optimizer != nil {
			msgs = a.optimizer.Optimize(ctx, allMsgs, lastInputTokens)
		}

		req := llm.Request{
			Messages: msgs,
			Tools:    a.tools.Definitions(),
		}
		if len(a.stickyBlocks) > 0 || a.skillsSection != "" {
			req.SystemSections = agentSystemSections(a.stickyBlocks, systemPrompt, a.skillsSection)
		} else {
			req.SystemPrompt = systemPrompt
		}
		// Append memories as an uncached section after all other system content.
		if a.memoryStore != nil {
			memBlock, err := BuildMemoriesSection(a.memoryStore, a.maxMemoryTokens)
			if err != nil {
				log.Warn("failed to build memories section", "error", err)
			} else if memBlock != "" {
				if len(req.SystemSections) > 0 {
					req.SystemSections = append(req.SystemSections, llm.SystemSection{
						Content:         memBlock,
						CacheCheckpoint: false,
					})
				} else {
					req.SystemPrompt = req.SystemPrompt + "\n\n" + memBlock
				}
			}
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
		lastInputTokens = resp.Usage.InputTokens

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

// agentSystemSections builds the system prompt as structured sections:
//
//   - Section 1 (checkpoint 1): sticky blocks — never changes
//   - Section 2: SOUL+IDENTITY personality string — rarely changes
//   - Section 3 (checkpoint 2): skills index — stable; cache checkpoint here
//     so memories (volatile, uncached) don't invalidate the skills cache
//
// When no skillsSection is provided the personality section becomes
// checkpoint 2 (matching the previous behaviour). Empty sections are omitted.
func agentSystemSections(sticky []StickyBlock, systemPrompt, skillsSection string) []llm.SystemSection {
	var sections []llm.SystemSection

	// Section 1: sticky blocks (cache checkpoint 1).
	var sb strings.Builder
	for _, s := range sticky {
		if c := strings.TrimSpace(s.Content); c != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c)
		}
	}
	if sb.Len() > 0 {
		sections = append(sections, llm.SystemSection{
			Content:         sb.String(),
			CacheCheckpoint: true,
		})
	}

	if skillsSection != "" {
		// With skills: personality has no checkpoint; skills section is checkpoint 2.
		if systemPrompt != "" {
			sections = append(sections, llm.SystemSection{
				Content:         systemPrompt,
				CacheCheckpoint: false,
			})
		}
		sections = append(sections, llm.SystemSection{
			Content:         skillsSection,
			CacheCheckpoint: true,
		})
	} else {
		// No skills: personality is checkpoint 2 (original behaviour).
		if systemPrompt != "" {
			sections = append(sections, llm.SystemSection{
				Content:         systemPrompt,
				CacheCheckpoint: true,
			})
		}
	}

	return sections
}
