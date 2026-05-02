package llm

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
)

// NewClientFromConfig constructs the appropriate Client based on agent config.
// credentialsPath is the path to credentials.toml; pass "" to use the default.
//
// The Config map contains inline values with env-var references already expanded:
//   - base_url, model, max_tokens, timeout_sec
//   - api_key: the actual token value (env-var expanded from $VAR pattern)
func NewClientFromConfig(cfg config.LLMConfig, credentialsPath string, logger *slog.Logger) (Client, error) {
	// Extract values from Config map (already env-var expanded by loadTOML).
	baseURL := getString(cfg.Config, "base_url", "")
	model := getString(cfg.Config, "model", "")
	maxTokens := getInt(cfg.Config, "max_tokens", 4096)
	timeout := time.Duration(getInt(cfg.Config, "timeout_sec", 60)) * time.Second
	apiFormat := cfg.Backend // "openai" or "anthropic"

	var tokenSource credentials.TokenSource

	// Get API key from config (expanded value).
	apiKey := getString(cfg.Config, "api_key", "")
	if apiKey != "" {
		tokenSource = credentials.NewFixedTokenSource(apiKey)
	} else if cfg.AuthProfile != "" {
		// Fall back to credentials.toml if no api_key in config.
		if credentialsPath == "" {
			credentialsPath = credentials.DefaultCredentialsPath()
		}
		var err error
		tokenSource, err = credentials.NewTokenSourceFromStore(credentialsPath, cfg.AuthProfile)
		if err != nil {
			return nil, fmt.Errorf("llm: load auth profile %q: %w", cfg.AuthProfile, err)
		}
	} else {
		return nil, fmt.Errorf("llm: no api_key in config and no auth_profile specified")
	}

	switch apiFormat {
	case "openai":
		return NewOpenAICompatClient(OpenAICompatConfig{
			BaseURL:     baseURL,
			TokenSource: tokenSource,
			Model:       model,
			MaxTokens:   maxTokens,
			Timeout:     timeout,
			Logger:      logger,
		})
	case "anthropic":
		promptCaching := cfg.PromptCaching == nil || *cfg.PromptCaching
		return NewAnthropicClient(AnthropicConfig{
			BaseURL:       baseURL,
			TokenSource:   tokenSource,
			Model:         model,
			MaxTokens:     maxTokens,
			Timeout:       timeout,
			Logger:        logger,
			PromptCaching: promptCaching,
		})
	default:
		return nil, fmt.Errorf("llm: unknown api_format %q (supported: openai, anthropic)", apiFormat)
	}
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
