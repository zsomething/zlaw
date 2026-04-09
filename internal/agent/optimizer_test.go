package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
)

// stubSummarizer returns a fixed summary message.
type stubSummarizer struct {
	called int
	text   string
}

func (s *stubSummarizer) Summarize(_ context.Context, _ []llm.Message) (llm.Message, error) {
	s.called++
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: []llm.ContentBlock{{Text: s.text}},
	}, nil
}

func largeTurnPair(n int) []llm.Message {
	text := strings.Repeat("x", n*4) // n*4 chars ≈ n tokens each
	return []llm.Message{
		textMsg(llm.RoleUser, text),
		textMsg(llm.RoleAssistant, text),
	}
}

func TestContextOptimizer_NoBudget(t *testing.T) {
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{}, nil, nil)
	msgs := largeTurnPair(1000)
	got := opt.Optimize(context.Background(), msgs, 0)
	if len(got) != len(msgs) {
		t.Fatalf("budget=0 should not modify messages")
	}
}

func TestContextOptimizer_WithinBudget(t *testing.T) {
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{TokenBudget: 100000}, nil, nil)
	msgs := largeTurnPair(10)
	got := opt.Optimize(context.Background(), msgs, 0)
	if len(got) != len(msgs) {
		t.Fatalf("within budget should not modify messages")
	}
}

func TestContextOptimizer_PruneOnlyWhenNoSummarizer(t *testing.T) {
	// Large first turn, small second turn. Budget only fits the second.
	large := strings.Repeat("a", 4000) // 1000 tokens
	msgs := []llm.Message{
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
		textMsg(llm.RoleUser, "short"),
		textMsg(llm.RoleAssistant, "ok"),
	}
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget: 10, // tight — forces prune
	}, nil, nil)
	got := opt.Optimize(context.Background(), msgs, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after prune, got %d", len(got))
	}
	if got[0].Content[0].Text != "short" {
		t.Fatalf("expected most recent turn preserved")
	}
}

func TestContextOptimizer_SummarizesBeforePruning(t *testing.T) {
	sum := &stubSummarizer{text: "[Summary of earlier conversation]\nThe user asked things."}

	large := strings.Repeat("b", 4000) // 1000 tokens each
	// Three turns: old1, old2, recent
	msgs := []llm.Message{
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
		textMsg(llm.RoleUser, "latest"),
		textMsg(llm.RoleAssistant, "reply"),
	}

	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget:        3000, // exceeded by full history
		SummarizeThreshold: 0.5, // trigger at 1500 tokens
		SummarizeTurns:     2,   // collapse 2 oldest turns
	}, sum, nil)

	got := opt.Optimize(context.Background(), msgs, 0)

	if sum.called == 0 {
		t.Fatal("expected summarizer to be called")
	}
	// Result: [summary_msg, recent_user, recent_assistant]
	if got[0].Content[0].Text != sum.text {
		t.Fatalf("expected summary as first message, got: %q", got[0].Content[0].Text)
	}
}

func TestContextOptimizer_FallsBackToPruneOnSummarizerError(t *testing.T) {
	errSum := &errorSummarizer{}
	large := strings.Repeat("c", 4000)
	msgs := []llm.Message{
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
		textMsg(llm.RoleUser, "latest"),
		textMsg(llm.RoleAssistant, "ok"),
	}
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget:        10,
		SummarizeThreshold: 0.5,
	}, errSum, nil)

	got := opt.Optimize(context.Background(), msgs, 0)
	// Should have pruned the first turn and kept the latest.
	if got[0].Content[0].Text != "latest" {
		t.Fatalf("expected prune fallback, got: %v", got[0])
	}
}

func TestContextOptimizer_KnownTokensTriggersOptimization(t *testing.T) {
	// First turn is large (high char estimate), second turn is short.
	// knownTokens says the full context is over budget; optimizer must prune.
	large := strings.Repeat("a", 4000) // 1000 char-estimated tokens
	msgs := []llm.Message{
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
		textMsg(llm.RoleUser, "latest"),
		textMsg(llm.RoleAssistant, "ok"),
	}
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget: 10, // tiny — only the short second turn fits
	}, nil, nil)

	// Pass knownTokens well above budget; optimizer should prune the first turn.
	got := opt.Optimize(context.Background(), msgs, 5000)
	if len(got) == len(msgs) {
		t.Fatal("expected optimizer to prune when knownTokens exceeds budget")
	}
}

func TestContextOptimizer_KnownTokensWithinBudgetSkipsOptimization(t *testing.T) {
	// Large messages by heuristic, but knownTokens says we're within budget.
	large := strings.Repeat("z", 80000) // ~20 000 estimated tokens
	msgs := []llm.Message{
		textMsg(llm.RoleUser, large),
		textMsg(llm.RoleAssistant, large),
	}
	opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget: 100000, // plenty of room
	}, nil, nil)

	got := opt.Optimize(context.Background(), msgs, 500) // real count well within budget
	if len(got) != len(msgs) {
		t.Fatal("expected no optimization when knownTokens is within budget")
	}
}

type errorSummarizer struct{}

func (e *errorSummarizer) Summarize(_ context.Context, _ []llm.Message) (llm.Message, error) {
	return llm.Message{}, errSummarizerFailed
}

var errSummarizerFailed = &summarizeError{}

type summarizeError struct{}

func (e *summarizeError) Error() string { return "summarizer failed" }
