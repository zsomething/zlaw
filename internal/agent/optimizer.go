package agent

import (
	"context"
	"log/slog"

	"github.com/chickenzord/zlaw/internal/llm"
)

// ContextOptimizerConfig controls the summarize→prune pipeline.
type ContextOptimizerConfig struct {
	// TokenBudget is the hard ceiling on estimated tokens sent per LLM call.
	// Zero disables all optimization.
	TokenBudget int

	// SummarizeThreshold is the fraction of TokenBudget at which summarization
	// is attempted before falling back to pruning (e.g. 0.8 = 80%).
	// Zero or negative disables summarization (prune-only mode).
	SummarizeThreshold float64

	// SummarizeTurns is the number of oldest turns to collapse per summarization
	// pass. Defaults to 10 when zero.
	SummarizeTurns int

	// PruneLevels is an ordered list of pruning strategies applied in sequence
	// after summarization. Each level is tried in turn; iteration stops as soon
	// as the estimated token count falls within TokenBudget.
	// Empty defaults to [drop_pairs] for backward compatibility.
	PruneLevels []PruneLevel
}

// ContextOptimizer applies the summarize→prune pipeline to a message slice.
// It is stateless and safe for concurrent use.
type ContextOptimizer struct {
	cfg        ContextOptimizerConfig
	summarizer Summarizer // nil = no summarization
	logger     *slog.Logger
}

// NewContextOptimizer returns a ContextOptimizer. summarizer may be nil to
// disable the summarization step (prune-only mode).
func NewContextOptimizer(cfg ContextOptimizerConfig, summarizer Summarizer, logger *slog.Logger) *ContextOptimizer {
	if cfg.SummarizeTurns <= 0 {
		cfg.SummarizeTurns = 10
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ContextOptimizer{cfg: cfg, summarizer: summarizer, logger: logger}
}

// Optimize returns a (possibly shorter) message slice that fits within the
// token budget. It applies, in order:
//
//  1. No-op if within budget.
//  2. Summarize oldest N turns if approaching the summarize threshold and a
//     summarizer is configured. Falls back to pruning if summarization fails.
//  3. Prune oldest turns as a final fallback.
//
// knownTokens is the actual input token count from the most recent API
// response. When > 0 it is used instead of the character-heuristic estimate,
// giving the optimizer a more accurate baseline. Pass 0 on the first call
// within a turn (before any response is available).
func (o *ContextOptimizer) Optimize(ctx context.Context, msgs []llm.Message, knownTokens int) []llm.Message {
	if o.cfg.TokenBudget <= 0 {
		return msgs
	}

	est := knownTokens
	if est <= 0 {
		est = EstimateTokens(msgs)
	}
	if est <= o.cfg.TokenBudget {
		return msgs // within budget, nothing to do
	}

	o.logger.Debug("context optimization triggered",
		"estimated_tokens", est, "budget", o.cfg.TokenBudget)

	// Step 1: summarize oldest turns if above the summarize threshold.
	summarizeAt := int(float64(o.cfg.TokenBudget) * o.cfg.SummarizeThreshold)
	if o.summarizer != nil && o.cfg.SummarizeThreshold > 0 && est > summarizeAt {
		if summarized, err := o.summarizeTurns(ctx, msgs); err != nil {
			o.logger.Warn("context summarization failed, falling back to prune", "error", err)
		} else {
			msgs = summarized
			est = EstimateTokens(msgs)
			o.logger.Debug("context summarized", "estimated_tokens", est)
		}
	}

	// Step 2: apply cascading prune levels if still over budget.
	if est > o.cfg.TokenBudget {
		msgs = o.applyPruneLevels(msgs)
	}

	return msgs
}

// applyPruneLevels iterates through the configured prune levels in order,
// applying each one and re-estimating tokens. Stops as soon as the estimate
// falls within budget. Falls back to drop_pairs when PruneLevels is empty.
func (o *ContextOptimizer) applyPruneLevels(msgs []llm.Message) []llm.Message {
	levels := o.cfg.PruneLevels
	if len(levels) == 0 {
		levels = []PruneLevel{PruneLevelDropPairs}
	}

	for _, level := range levels {
		if EstimateTokens(msgs) <= o.cfg.TokenBudget {
			break
		}
		before := len(msgs)
		switch level {
		case PruneLevelStripThinking:
			msgs = StripThinking(msgs)
			o.logger.Debug("prune level applied", "level", level,
				"estimated_tokens", EstimateTokens(msgs))
		case PruneLevelStripToolResults:
			msgs = StripToolResults(msgs)
			o.logger.Debug("prune level applied", "level", level,
				"estimated_tokens", EstimateTokens(msgs))
		case PruneLevelDropPairs:
			msgs = PruneMessages(msgs, o.cfg.TokenBudget)
			o.logger.Debug("prune level applied", "level", level,
				"dropped_messages", before-len(msgs))
		default:
			o.logger.Warn("unknown prune level, skipping", "level", level)
		}
	}

	return msgs
}

// summarizeTurns collapses the oldest SummarizeTurns turns into a single
// summary message and returns the resulting message slice.
func (o *ContextOptimizer) summarizeTurns(ctx context.Context, msgs []llm.Message) ([]llm.Message, error) {
	turns := splitTurns(msgs)
	if len(turns) <= 1 {
		return msgs, nil // nothing to summarize without losing the current turn
	}

	// Collect the oldest N turns for summarization.
	n := o.cfg.SummarizeTurns
	if n >= len(turns) {
		n = len(turns) - 1 // always keep the most recent turn
	}
	toSummarize := flattenTurns(turns[:n])
	remaining := flattenTurns(turns[n:])

	summary, err := o.summarizer.Summarize(ctx, toSummarize)
	if err != nil {
		return nil, err
	}

	// Prepend the summary message before the remaining turns.
	result := make([]llm.Message, 0, 1+len(remaining))
	result = append(result, summary)
	result = append(result, remaining...)
	return result, nil
}

// splitTurns groups msgs into turns. Each turn starts at a user message and
// includes all following non-user messages up to (but not including) the next
// user message.
func splitTurns(msgs []llm.Message) [][]llm.Message {
	var turns [][]llm.Message
	var current []llm.Message

	for _, m := range msgs {
		if m.Role == llm.RoleUser && len(current) > 0 {
			turns = append(turns, current)
			current = nil
		}
		current = append(current, m)
	}
	if len(current) > 0 {
		turns = append(turns, current)
	}
	return turns
}

// flattenTurns converts a slice of turns back to a flat message slice.
func flattenTurns(turns [][]llm.Message) []llm.Message {
	var out []llm.Message
	for _, t := range turns {
		out = append(out, t...)
	}
	return out
}
