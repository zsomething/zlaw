package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/chickenzord/zlaw/internal/llm/auth"
)

const anthropicVersion = "2023-06-01"

// AnthropicConfig holds settings for the Anthropic Messages API backend.
type AnthropicConfig struct {
	BaseURL     string
	TokenSource auth.TokenSource
	Model       string
	MaxTokens   int
	Timeout     time.Duration
	Logger      *slog.Logger

	// PromptCaching enables prompt caching for the system prompt by tagging it
	// with cache_control {"type":"ephemeral"}. Reduces input token cost and
	// latency on cache hits. Requires the anthropic-beta header.
	PromptCaching bool
}

type anthropicClient struct {
	cfg    AnthropicConfig
	client *http.Client
}

// NewAnthropicClient creates a StreamingClient for the Anthropic Messages API.
func NewAnthropicClient(cfg AnthropicConfig) (Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("anthropic: BaseURL is required")
	}
	if cfg.TokenSource == nil {
		return nil, fmt.Errorf("anthropic: TokenSource is required")
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &anthropicClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

func (c *anthropicClient) Complete(ctx context.Context, req Request) (Response, error) {
	return c.CompleteStream(ctx, req, nil)
}

func (c *anthropicClient) CompleteStream(ctx context.Context, req Request, handler StreamHandler) (Response, error) {
	apiKey, err := c.cfg.TokenSource.Token(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: resolve token: %w", err)
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = c.cfg.MaxTokens
	}

	areq, err := toAnthropicRequest(req, c.cfg.Model, maxTokens, handler != nil, c.cfg.PromptCaching)
	if err != nil {
		return Response{}, err
	}

	body, err := json.Marshal(areq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	hasSystemContent := req.SystemPrompt != "" || len(req.SystemSections) > 0
	if c.cfg.PromptCaching && hasSystemContent {
		httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
	}
	if handler != nil {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Response{}, fmt.Errorf("anthropic: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return Response{}, fmt.Errorf("%w: HTTP 429 from Anthropic", ErrRateLimit)
	}
	if resp.StatusCode == 529 {
		return Response{}, fmt.Errorf("%w: HTTP 529 from Anthropic", ErrOverloaded)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return Response{}, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, string(data))
	}

	if handler != nil {
		return parseAnthropicStream(resp.Body, handler)
	}
	return parseAnthropicResponse(resp.Body)
}

// ── wire types ──────────────────────────────────────────────────────────────

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    json.RawMessage    `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
}

// anthropicSystemBlock is a single block in the system array (used for caching).
type anthropicSystemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ── request translation ──────────────────────────────────────────────────────

func toAnthropicRequest(req Request, model string, maxTokens int, stream bool, promptCaching bool) (anthropicRequest, error) {
	msgs, err := toAnthropicMessages(req.Messages)
	if err != nil {
		return anthropicRequest{}, err
	}

	var systemJSON json.RawMessage
	if len(req.SystemSections) > 0 {
		// Structured sections path: each section becomes one block; sections
		// with CacheCheckpoint=true get cache_control when caching is enabled.
		var blocks []anthropicSystemBlock
		for _, sec := range req.SystemSections {
			if sec.Content == "" {
				continue
			}
			blk := anthropicSystemBlock{Type: "text", Text: sec.Content}
			if promptCaching && sec.CacheCheckpoint {
				blk.CacheControl = json.RawMessage(`{"type":"ephemeral"}`)
			}
			blocks = append(blocks, blk)
		}
		if len(blocks) > 0 {
			systemJSON, err = json.Marshal(blocks)
			if err != nil {
				return anthropicRequest{}, fmt.Errorf("anthropic: marshal system sections: %w", err)
			}
		}
	} else if req.SystemPrompt != "" {
		// Legacy single-string path.
		if promptCaching {
			blocks := []anthropicSystemBlock{{
				Type:         "text",
				Text:         req.SystemPrompt,
				CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
			}}
			systemJSON, err = json.Marshal(blocks)
			if err != nil {
				return anthropicRequest{}, fmt.Errorf("anthropic: marshal system blocks: %w", err)
			}
		} else {
			systemJSON, err = json.Marshal(req.SystemPrompt)
			if err != nil {
				return anthropicRequest{}, fmt.Errorf("anthropic: marshal system prompt: %w", err)
			}
		}
	}

	areq := anthropicRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    systemJSON,
		Messages:  msgs,
		Stream:    stream,
	}

	if len(req.Tools) > 0 {
		tools, err := toAnthropicTools(req.Tools)
		if err != nil {
			return anthropicRequest{}, err
		}
		areq.Tools = tools
	}

	return areq, nil
}

func toAnthropicMessages(msgs []Message) ([]anthropicMessage, error) {
	var out []anthropicMessage
	for _, m := range msgs {
		am, err := toAnthropicMessage(m)
		if err != nil {
			return nil, err
		}
		out = append(out, am)
	}
	return out, nil
}

func toAnthropicMessage(m Message) (anthropicMessage, error) {
	switch m.Role {
	case RoleUser:
		var blocks []anthropicContentBlock
		for _, b := range m.Content {
			if b.Text != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: b.Text})
			}
			if b.ToolResult != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: b.ToolResult.ToolUseID,
					Content:   b.ToolResult.Content,
					IsError:   b.ToolResult.IsError,
				})
			}
		}
		return anthropicMessage{Role: "user", Content: blocks}, nil

	case RoleAssistant:
		var blocks []anthropicContentBlock
		for _, b := range m.Content {
			if b.Text != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: b.Text})
			}
			if b.ToolUse != nil {
				input := b.ToolUse.Input
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    b.ToolUse.ID,
					Name:  b.ToolUse.Name,
					Input: input,
				})
			}
		}
		return anthropicMessage{Role: "assistant", Content: blocks}, nil

	case RoleTool:
		// Tool results belong in user messages for Anthropic.
		// The agent layer uses RoleTool; we map it to user with tool_result blocks.
		var blocks []anthropicContentBlock
		for _, b := range m.Content {
			if b.ToolResult != nil {
				blocks = append(blocks, anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: b.ToolResult.ToolUseID,
					Content:   b.ToolResult.Content,
					IsError:   b.ToolResult.IsError,
				})
			}
		}
		return anthropicMessage{Role: "user", Content: blocks}, nil

	default:
		return anthropicMessage{}, fmt.Errorf("anthropic: unsupported role %q", m.Role)
	}
}

func toAnthropicTools(tools []ToolDefinition) ([]anthropicTool, error) {
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return out, nil
}

// ── response translation ─────────────────────────────────────────────────────

func parseAnthropicResponse(body io.Reader) (Response, error) {
	var ar anthropicResponse
	if err := json.NewDecoder(body).Decode(&ar); err != nil {
		return Response{}, fmt.Errorf("anthropic: decode response: %w", err)
	}
	return fromAnthropicResponse(ar)
}

func fromAnthropicResponse(ar anthropicResponse) (Response, error) {
	var blocks []ContentBlock
	for _, b := range ar.Content {
		switch b.Type {
		case "text":
			blocks = append(blocks, ContentBlock{Text: b.Text})
		case "tool_use":
			blocks = append(blocks, ContentBlock{ToolUse: &ToolUse{
				ID:    b.ID,
				Name:  b.Name,
				Input: []byte(b.Input),
			}})
		}
	}
	return Response{
		Message:    Message{Role: RoleAssistant, Content: blocks},
		StopReason: ar.StopReason,
		Usage: Usage{
			InputTokens:  ar.Usage.InputTokens,
			OutputTokens: ar.Usage.OutputTokens,
		},
	}, nil
}

// ── streaming ────────────────────────────────────────────────────────────────

// streamBlock tracks an in-progress content block during streaming.
type streamBlock struct {
	blockType  string
	text       strings.Builder
	toolID     string
	toolName   string
	toolInput  strings.Builder
}

func parseAnthropicStream(body io.Reader, handler StreamHandler) (Response, error) {
	scanner := bufio.NewScanner(body)

	blocks := map[int]*streamBlock{}
	var inputTokens, outputTokens int
	var stopReason string

	var eventType string
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch eventType {
		case "message_start":
			var e struct {
				Message struct {
					Usage anthropicUsage `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &e); err == nil {
				inputTokens = e.Message.Usage.InputTokens
			}

		case "content_block_start":
			var e struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"`
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &e); err == nil {
				blocks[e.Index] = &streamBlock{
					blockType: e.ContentBlock.Type,
					toolID:    e.ContentBlock.ID,
					toolName:  e.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			var e struct {
				Index int `json:"index"`
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				continue
			}
			blk := blocks[e.Index]
			if blk == nil {
				continue
			}
			switch e.Delta.Type {
			case "text_delta":
				blk.text.WriteString(e.Delta.Text)
				if handler != nil && e.Delta.Text != "" {
					handler(e.Delta.Text)
				}
			case "input_json_delta":
				blk.toolInput.WriteString(e.Delta.PartialJSON)
			}

		case "message_delta":
			var e struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage anthropicUsage `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &e); err == nil {
				stopReason = e.Delta.StopReason
				outputTokens = e.Usage.OutputTokens
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, fmt.Errorf("anthropic: read stream: %w", err)
	}

	// Build content blocks in index order.
	var contentBlocks []ContentBlock
	for i := 0; i < len(blocks); i++ {
		blk, ok := blocks[i]
		if !ok {
			continue
		}
		switch blk.blockType {
		case "text":
			if t := blk.text.String(); t != "" {
				contentBlocks = append(contentBlocks, ContentBlock{Text: t})
			}
		case "tool_use":
			input := []byte(blk.toolInput.String())
			if len(input) == 0 {
				input = []byte("{}")
			}
			contentBlocks = append(contentBlocks, ContentBlock{ToolUse: &ToolUse{
				ID:    blk.toolID,
				Name:  blk.toolName,
				Input: input,
			}})
		}
	}

	return Response{
		Message:    Message{Role: RoleAssistant, Content: contentBlocks},
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}, nil
}
