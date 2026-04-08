package agent_test

import (
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/llm"
)

func textMsg(role llm.Role, text string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Text: text}}}
}

func TestPruneMessages_NoBudget(t *testing.T) {
	msgs := []llm.Message{
		textMsg(llm.RoleUser, "hello"),
		textMsg(llm.RoleAssistant, "hi"),
	}
	got := agent.PruneMessages(msgs, 0)
	if len(got) != len(msgs) {
		t.Fatalf("budget=0 should not prune: got %d messages", len(got))
	}
}

func TestPruneMessages_WithinBudget(t *testing.T) {
	msgs := []llm.Message{
		textMsg(llm.RoleUser, "hi"),
		textMsg(llm.RoleAssistant, "hello"),
	}
	got := agent.PruneMessages(msgs, 10000)
	if len(got) != len(msgs) {
		t.Fatalf("within budget should not prune: got %d messages", len(got))
	}
}

func TestPruneMessages_DropsOldestTurn(t *testing.T) {
	// Two turns; first is large, second is small.
	largeTxt := strings.Repeat("a", 4000) // ~1000 tokens
	msgs := []llm.Message{
		textMsg(llm.RoleUser, largeTxt),
		textMsg(llm.RoleAssistant, largeTxt),
		textMsg(llm.RoleUser, "short"),
		textMsg(llm.RoleAssistant, "reply"),
	}
	// Budget of 10 tokens — forces the first turn to be dropped.
	got := agent.PruneMessages(msgs, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages after pruning, got %d", len(got))
	}
	if got[0].Role != llm.RoleUser || got[0].Content[0].Text != "short" {
		t.Fatalf("expected most recent turn preserved, got: %v", got[0])
	}
}

func TestPruneMessages_PreservesLastTurn(t *testing.T) {
	// Even with zero budget, the last turn must survive.
	largeTxt := strings.Repeat("x", 40000)
	msgs := []llm.Message{
		textMsg(llm.RoleUser, largeTxt),
		textMsg(llm.RoleAssistant, largeTxt),
	}
	got := agent.PruneMessages(msgs, 1) // impossibly tight budget
	if len(got) != 2 {
		t.Fatalf("last turn must never be dropped: got %d messages", len(got))
	}
}

func TestPruneMessages_MultipleToolTurns(t *testing.T) {
	// Turn 1: user + assistant(tool) + tool_result
	// Turn 2: user + assistant
	largeTxt := strings.Repeat("b", 4000)
	msgs := []llm.Message{
		textMsg(llm.RoleUser, largeTxt),
		textMsg(llm.RoleAssistant, largeTxt),
		textMsg(llm.RoleTool, largeTxt),
		textMsg(llm.RoleUser, "ok"),
		textMsg(llm.RoleAssistant, "done"),
	}
	got := agent.PruneMessages(msgs, 10)
	// First 3 messages (turn 1) should be dropped.
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d: %v", len(got), got)
	}
	if got[0].Content[0].Text != "ok" {
		t.Fatalf("unexpected leading message after prune: %v", got[0])
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []llm.Message{
		textMsg(llm.RoleUser, strings.Repeat("x", 400)), // 400 chars = 100 tokens
	}
	est := agent.EstimateTokens(msgs)
	if est != 100 {
		t.Fatalf("expected 100, got %d", est)
	}
}
