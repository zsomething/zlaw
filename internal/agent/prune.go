package agent

import "github.com/zsomething/zlaw/internal/llm"

// PruneLevel identifies a single context-pruning strategy.
type PruneLevel string

const (
	// PruneLevelStripThinking removes thinking blocks from old turns.
	PruneLevelStripThinking PruneLevel = "strip_thinking"
	// PruneLevelStripToolResults clears tool-result content in old turns (keeps structure).
	PruneLevelStripToolResults PruneLevel = "strip_tool_results"
	// PruneLevelDropPairs drops the oldest user+assistant turn(s) until within budget.
	PruneLevelDropPairs PruneLevel = "drop_pairs"
)

// EstimateTokens returns a rough token count for a message slice.
// It uses the heuristic of 1 token per 4 characters across all content.
func EstimateTokens(msgs []llm.Message) int {
	var chars int
	for _, m := range msgs {
		for _, b := range m.Content {
			chars += len(b.Text) + len(b.Thinking)
			if b.ToolUse != nil {
				chars += len(b.ToolUse.Name) + len(b.ToolUse.Input)
			}
			if b.ToolResult != nil {
				chars += len(b.ToolResult.Content)
			}
		}
	}
	return chars / 4
}

// lastTurnStart returns the index of the first message in the most recent turn
// (i.e. the last user message). Returns 0 if there is only one turn.
func lastTurnStart(msgs []llm.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleUser {
			return i
		}
	}
	return 0
}

// StripThinking removes thinking blocks from all messages in old turns
// (everything before the most recent turn). Returns a new slice; original
// messages are not mutated.
func StripThinking(msgs []llm.Message) []llm.Message {
	cutoff := lastTurnStart(msgs)
	if cutoff == 0 {
		return msgs // single turn — nothing to strip
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := 0; i < cutoff; i++ {
		m := msgs[i]
		var blocks []llm.ContentBlock
		for _, b := range m.Content {
			if b.Thinking != "" && b.Text == "" && b.ToolUse == nil && b.ToolResult == nil {
				continue // pure thinking block — drop it
			}
			if b.Thinking != "" {
				b.Thinking = "" // mixed block — clear thinking, keep rest
			}
			blocks = append(blocks, b)
		}
		out[i] = llm.Message{Role: m.Role, Content: blocks}
	}
	return out
}

// StripToolResults clears the Content field of every ToolResult block in old
// turns, keeping the block structure (ToolUseID) so the conversation remains
// valid. Returns a new slice; original messages are not mutated.
func StripToolResults(msgs []llm.Message) []llm.Message {
	cutoff := lastTurnStart(msgs)
	if cutoff == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	for i := 0; i < cutoff; i++ {
		m := msgs[i]
		var blocks []llm.ContentBlock
		for _, b := range m.Content {
			if b.ToolResult != nil && b.ToolResult.Content != "" {
				tr := *b.ToolResult
				tr.Content = ""
				b.ToolResult = &tr
			}
			blocks = append(blocks, b)
		}
		out[i] = llm.Message{Role: m.Role, Content: blocks}
	}
	return out
}

// pruneMessages removes the oldest conversation turns from msgs until the
// estimated token count falls within budget (or only one turn remains).
//
// A "turn" begins at a user message and includes all following non-user
// messages up to (but not including) the next user message. The most recent
// turn is never dropped, ensuring the current user input is always present.
//
// If budget is zero or negative, msgs is returned unchanged.
func PruneMessages(msgs []llm.Message, budget int) []llm.Message {
	if budget <= 0 || EstimateTokens(msgs) <= budget {
		return msgs
	}

	// Collect the start index of every user turn.
	var turnStarts []int
	for i, m := range msgs {
		if m.Role == llm.RoleUser {
			turnStarts = append(turnStarts, i)
		}
	}

	// Drop the oldest turn repeatedly until within budget or only one turn left.
	for len(turnStarts) > 1 && EstimateTokens(msgs) > budget {
		msgs = msgs[turnStarts[1]:]
		// Recalculate offsets after slicing.
		next := turnStarts[1]
		for i := range turnStarts {
			turnStarts[i] -= next
		}
		turnStarts = turnStarts[1:]
	}

	return msgs
}
