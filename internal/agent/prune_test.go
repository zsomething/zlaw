package agent_test

import (
	"strings"
	"testing"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/llm"
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

func thinkingMsg(role llm.Role, text, thinking string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Text: text, Thinking: thinking}}}
}

func pureThinkingMsg(role llm.Role, thinking string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Thinking: thinking}}}
}

func toolResultMsg(toolUseID, content string) llm.Message {
	return llm.Message{
		Role: llm.RoleTool,
		Content: []llm.ContentBlock{
			{ToolResult: &llm.ToolResult{ToolUseID: toolUseID, Content: content}},
		},
	}
}

func TestStripThinking_SingleTurn_NoOp(t *testing.T) {
	// Only one turn — nothing old to strip.
	msgs := []llm.Message{
		thinkingMsg(llm.RoleUser, "hi", "thinking..."),
		thinkingMsg(llm.RoleAssistant, "reply", "more thinking"),
	}
	got := agent.StripThinking(msgs)
	if got[0].Content[0].Thinking != "thinking..." {
		t.Fatal("single turn: thinking should be preserved")
	}
}

func TestStripThinking_StripsOldTurns(t *testing.T) {
	msgs := []llm.Message{
		pureThinkingMsg(llm.RoleAssistant, "plan stuff"),               // old: pure thinking block
		thinkingMsg(llm.RoleAssistant, "answer", "some thought"),       // old: mixed block
		textMsg(llm.RoleUser, "next question"),                         // start of recent turn
		thinkingMsg(llm.RoleAssistant, "final answer", "keep this"),    // recent
	}
	got := agent.StripThinking(msgs)

	// Old pure-thinking block should be dropped entirely.
	if got[0].Content != nil && len(got[0].Content) > 0 {
		for _, b := range got[0].Content {
			if b.Thinking != "" {
				t.Fatalf("old pure thinking block should have been removed")
			}
		}
	}

	// Old mixed block: Thinking cleared, Text preserved.
	if got[1].Content[0].Thinking != "" {
		t.Fatalf("old mixed block thinking should be cleared, got: %q", got[1].Content[0].Thinking)
	}
	if got[1].Content[0].Text != "answer" {
		t.Fatalf("old mixed block text should be preserved, got: %q", got[1].Content[0].Text)
	}

	// Recent turn untouched.
	if got[3].Content[0].Thinking != "keep this" {
		t.Fatalf("recent turn thinking should be preserved")
	}
}

func TestStripToolResults_SingleTurn_NoOp(t *testing.T) {
	msgs := []llm.Message{
		textMsg(llm.RoleUser, "run it"),
		toolResultMsg("tu1", "big output"),
	}
	got := agent.StripToolResults(msgs)
	if got[1].Content[0].ToolResult.Content != "big output" {
		t.Fatal("single turn: tool result should be preserved")
	}
}

func TestStripToolResults_ClearsOldContent(t *testing.T) {
	msgs := []llm.Message{
		textMsg(llm.RoleUser, "run it"),
		toolResultMsg("tu1", "big output"), // old turn
		textMsg(llm.RoleUser, "next"),      // start of recent turn
		toolResultMsg("tu2", "keep this"), // recent
	}
	got := agent.StripToolResults(msgs)

	// Old tool result content cleared, ToolUseID preserved.
	if got[1].Content[0].ToolResult.Content != "" {
		t.Fatalf("old tool result content should be cleared, got: %q", got[1].Content[0].ToolResult.Content)
	}
	if got[1].Content[0].ToolResult.ToolUseID != "tu1" {
		t.Fatal("old tool result ToolUseID should be preserved")
	}

	// Recent turn tool result untouched.
	if got[3].Content[0].ToolResult.Content != "keep this" {
		t.Fatal("recent tool result content should be preserved")
	}
}
