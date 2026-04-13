package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/zsomething/zlaw/internal/llm"
)

// Summarizer compresses a slice of messages into a single summary message.
type Summarizer interface {
	Summarize(ctx context.Context, msgs []llm.Message) (llm.Message, error)
}

// LLMSummarizer uses an llm.Client to produce a natural-language summary of
// a conversation fragment. The returned message has role=assistant and contains
// a [Summary] block that describes the compressed turns.
type LLMSummarizer struct {
	client llm.Client
}

// NewLLMSummarizer returns a Summarizer backed by client.
func NewLLMSummarizer(client llm.Client) *LLMSummarizer {
	return &LLMSummarizer{client: client}
}

const summarizeSystemPrompt = `You are a conversation compressor. Your only job is to produce a concise factual summary of the conversation fragment you receive. Rules:
- Write in third person (e.g. "The user asked...", "The assistant explained...").
- Preserve all facts, decisions, file paths, code snippets, and tool results that may be needed later.
- Do not add commentary or opinions.
- Output only the summary text, no preamble.`

// Summarize sends msgs to the LLM and returns an assistant message containing
// the summary prefixed with "[Summary of earlier conversation]\n".
func (s *LLMSummarizer) Summarize(ctx context.Context, msgs []llm.Message) (llm.Message, error) {
	if len(msgs) == 0 {
		return llm.Message{}, fmt.Errorf("summarize: no messages to summarize")
	}

	// Build a text representation of the fragment for the summarizer.
	var b strings.Builder
	for _, m := range msgs {
		role := string(m.Role)
		for _, block := range m.Content {
			switch {
			case block.Text != "":
				fmt.Fprintf(&b, "[%s]: %s\n", role, block.Text)
			case block.ToolUse != nil:
				fmt.Fprintf(&b, "[%s/tool_call]: %s(%s)\n", role, block.ToolUse.Name, block.ToolUse.Input)
			case block.ToolResult != nil:
				fmt.Fprintf(&b, "[tool_result]: %s\n", block.ToolResult.Content)
			case block.Thinking != "":
				// Skip thinking blocks — they aren't observable content.
			}
		}
	}

	req := llm.Request{
		SystemPrompt: summarizeSystemPrompt,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{{Text: b.String()}},
			},
		},
	}

	resp, err := s.client.Complete(ctx, req)
	if err != nil {
		return llm.Message{}, fmt.Errorf("summarize: llm call: %w", err)
	}

	summary := "[Summary of earlier conversation]\n" + strings.TrimSpace(resp.Message.TextContent())
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: []llm.ContentBlock{{Text: summary}},
	}, nil
}
