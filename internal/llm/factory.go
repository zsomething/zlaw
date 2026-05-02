package llm

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
)

// NewClientFromConfig constructs the appropriate Client based on agent config.
//
// Extracts from:
//   - cfg.ClientConfig: base_url, api_key
//   - cfg.Model: model name
//   - cfg.ModelConfig: max_tokens, timeout_sec, prompt_caching, etc.
//
// Backend dispatch by cfg.Backend ("openai", "anthropic").
func NewClientFromConfig(cfg config.LLMConfig, credentialsPath string, logger *slog.Logger) (Client, error) {
	backend := cfg.Backend

	// Get API key from ClientConfig.
	apiKey := getString(cfg.ClientConfig, "api_key", "")
	var tokenSource credentials.TokenSource
	if apiKey != "" {
		tokenSource = credentials.NewFixedTokenSource(apiKey)
	} else {
		return nil, fmt.Errorf("llm: api_key required in client_config")
	}

	switch backend {
	case "openai":
		return NewOpenAICompatClientFromConfig(cfg, tokenSource, logger)
	case "anthropic":
		return NewAnthropicClientFromConfig(cfg, tokenSource, logger)
	default:
		return nil, fmt.Errorf("llm: unknown backend %q (supported: openai, anthropic)", backend)
	}
}

// NewOpenAICompatClientFromConfig creates an OpenAI-compatible client.
func NewOpenAICompatClientFromConfig(cfg config.LLMConfig, token credentials.TokenSource, logger *slog.Logger) (Client, error) {
	return NewOpenAICompatClient(OpenAICompatConfig{
		BaseURL:     getString(cfg.ClientConfig, "base_url", ""),
		TokenSource: token,
		Model:       cfg.Model,
		MaxTokens:   getInt(cfg.ModelConfig, "max_tokens", 4096),
		Timeout:     getDuration(cfg.ModelConfig, "timeout_sec", 60),
		Logger:      logger,
	})
}

// NewAnthropicClientFromConfig creates an Anthropic client.
func NewAnthropicClientFromConfig(cfg config.LLMConfig, token credentials.TokenSource, logger *slog.Logger) (Client, error) {
	return NewAnthropicClient(AnthropicConfig{
		BaseURL:       getString(cfg.ClientConfig, "base_url", ""),
		TokenSource:   token,
		Model:         cfg.Model,
		MaxTokens:     getInt(cfg.ModelConfig, "max_tokens", 4096),
		Timeout:       getDuration(cfg.ModelConfig, "timeout_sec", 60),
		Logger:        logger,
		PromptCaching: getBool(cfg.ModelConfig, "prompt_caching", true),
	})
}

// getString extracts a string from a config map, with fallback.
func getString(cfg map[string]any, key, fallback string) string {
	if v, ok := cfg[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

// getInt extracts an int from a config map, with fallback.
func getInt(cfg map[string]any, key string, fallback int) int {
	if v, ok := cfg[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return fallback
}

// getDuration extracts a duration (seconds) from a config map, with fallback.
func getDuration(cfg map[string]any, key string, fallbackSec int) time.Duration {
	return time.Duration(getInt(cfg, key, fallbackSec)) * time.Second
}

// getBool extracts a bool from a config map, with fallback.
func getBool(cfg map[string]any, key string, fallback bool) bool {
	if v, ok := cfg[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}
