package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
	"github.com/chickenzord/zlaw/internal/llm/auth"
)

// OpenAICompatConfig holds settings for any OpenAI-compatible backend.
type OpenAICompatConfig struct {
	BaseURL     string
	TokenSource auth.TokenSource
	Model       string
	MaxTokens   int
	Timeout     time.Duration
	MaxRetries  int
	Logger      *slog.Logger
}

type openAICompatClient struct {
	cfg    OpenAICompatConfig
	client *openai.Client
}

// NewOpenAICompatClient creates a Client for any OpenAI-compatible API endpoint.
func NewOpenAICompatClient(cfg OpenAICompatConfig) (Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai_compat: BaseURL is required")
	}
	if cfg.TokenSource == nil {
		return nil, fmt.Errorf("openai_compat: TokenSource is required")
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}

	c := openai.NewClient(
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey("placeholder"), // overridden per-request via WithAPIKey
		option.WithMaxRetries(cfg.MaxRetries),
	)
	return &openAICompatClient{cfg: cfg, client: &c}, nil
}

func (c *openAICompatClient) Complete(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	token, err := c.cfg.TokenSource.Token(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("openai_compat: resolve token: %w", err)
	}

	params, err := toOpenAIParams(req, c.cfg.Model, c.cfg.MaxTokens)
	if err != nil {
		return Response{}, err
	}

	completion, err := c.client.Chat.Completions.New(ctx, params, option.WithAPIKey(token))
	if err != nil {
		if isHTTP429(err) {
			return Response{}, fmt.Errorf("%w: %w", ErrRateLimit, err)
		}
		return Response{}, fmt.Errorf("openai_compat complete: %w", err)
	}

	return fromOpenAICompletion(completion)
}

func toOpenAIParams(req Request, model string, defaultMaxTokens int) (openai.ChatCompletionNewParams, error) {
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	msgs, err := toOpenAIMessages(req)
	if err != nil {
		return openai.ChatCompletionNewParams{}, err
	}

	params := openai.ChatCompletionNewParams{
		Model:     openai.ChatModel(model),
		Messages:  msgs,
		MaxTokens: openai.Int(int64(maxTokens)),
	}

	if len(req.Tools) > 0 {
		tools, err := toOpenAITools(req.Tools)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		params.Tools = tools
	}

	return params, nil
}

func toOpenAIMessages(req Request) ([]openai.ChatCompletionMessageParamUnion, error) {
	var msgs []openai.ChatCompletionMessageParamUnion

	if req.SystemPrompt != "" {
		msgs = append(msgs, openai.SystemMessage(req.SystemPrompt))
	}

	for _, m := range req.Messages {
		switch m.Role {
		case RoleUser:
			msgs = append(msgs, openai.UserMessage(m.TextContent()))

		case RoleAssistant:
			toolUses := m.ToolUses()
			if len(toolUses) > 0 {
				calls := make([]openai.ChatCompletionMessageToolCallParam, 0, len(toolUses))
				for _, tu := range toolUses {
					calls = append(calls, openai.ChatCompletionMessageToolCallParam{
						ID: tu.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tu.Name,
							Arguments: string(tu.Input),
						},
					})
				}
				msgs = append(msgs, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						ToolCalls: calls,
					},
				})
			} else {
				msgs = append(msgs, openai.AssistantMessage(m.TextContent()))
			}

		case RoleTool:
			for _, b := range m.Content {
				if b.ToolResult == nil {
					continue
				}
				msgs = append(msgs, openai.ToolMessage(b.ToolResult.Content, b.ToolResult.ToolUseID))
			}

		default:
			return nil, fmt.Errorf("unsupported role: %s", m.Role)
		}
	}
	return msgs, nil
}

func toOpenAITools(tools []ToolDefinition) ([]openai.ChatCompletionToolParam, error) {
	out := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		var params shared.FunctionParameters
		if len(t.InputSchema) > 0 {
			if err := json.Unmarshal(t.InputSchema, &params); err != nil {
				return nil, fmt.Errorf("unmarshal schema for tool %s: %w", t.Name, err)
			}
		} else {
			params = shared.FunctionParameters{"type": "object", "properties": map[string]interface{}{}}
		}
		out = append(out, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  params,
			},
		})
	}
	return out, nil
}

func fromOpenAICompletion(c *openai.ChatCompletion) (Response, error) {
	if len(c.Choices) == 0 {
		return Response{}, fmt.Errorf("openai_compat: empty choices in response")
	}
	choice := c.Choices[0]

	var blocks []ContentBlock

	if choice.Message.Content != "" {
		blocks = append(blocks, ContentBlock{Text: choice.Message.Content})
	}

	for _, tc := range choice.Message.ToolCalls {
		blocks = append(blocks, ContentBlock{ToolUse: &ToolUse{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: []byte(tc.Function.Arguments),
		}})
	}

	return Response{
		Message: Message{
			Role:    RoleAssistant,
			Content: blocks,
		},
		StopReason: normalizeStopReason(string(choice.FinishReason)),
		Usage: Usage{
			InputTokens:  int(c.Usage.PromptTokens),
			OutputTokens: int(c.Usage.CompletionTokens),
		},
	}, nil
}

// normalizeStopReason maps OpenAI finish reasons to the canonical values used
// by the agent loop ("end_turn", "tool_use", "max_tokens").
func normalizeStopReason(r string) string {
	switch r {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return r
	}
}

func isHTTP429(err error) bool {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}
