package agent

import "github.com/chickenzord/zlaw/internal/llm"

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
