package llm

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm/auth"
)

// NewClientFromConfig constructs the appropriate Client based on agent config.
// credentialsPath is the path to credentials.toml; pass "" to use the default.
//
// Resolution order:
//  1. Look up the named preset from cfg.Backend.
//  2. Apply any per-field overrides from cfg (APIFormat, BaseURL).
//  3. Dispatch to the matching Client implementation by APIFormat.
func NewClientFromConfig(cfg config.LLMConfig, credentialsPath string, logger *slog.Logger) (Client, error) {
	if credentialsPath == "" {
		credentialsPath = auth.DefaultCredentialsPath()
	}

	src, err := auth.NewTokenSourceFromStore(credentialsPath, cfg.AuthProfile)
	if err != nil {
		return nil, fmt.Errorf("llm: load auth profile %q: %w", cfg.AuthProfile, err)
	}

	preset, err := LookupPreset(cfg.Backend)
	if err != nil {
		return nil, err
	}

	// Apply per-config overrides.
	baseURL := preset.BaseURL
	if cfg.BaseURL != "" {
		baseURL = cfg.BaseURL
	}
	apiFormat := preset.APIFormat
	if cfg.APIFormat != "" {
		apiFormat = APIFormat(cfg.APIFormat)
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second

	switch apiFormat {
	case APIFormatOpenAI:
		return NewOpenAICompatClient(OpenAICompatConfig{
			BaseURL:     baseURL,
			TokenSource: src,
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Timeout:     timeout,
			Logger:      logger,
		})
	case APIFormatAnthropic:
		promptCaching := cfg.PromptCaching == nil || *cfg.PromptCaching
		return NewAnthropicClient(AnthropicConfig{
			BaseURL:       baseURL,
			TokenSource:   src,
			Model:         cfg.Model,
			MaxTokens:     cfg.MaxTokens,
			Timeout:       timeout,
			Logger:        logger,
			PromptCaching: promptCaching,
		})
	default:
		return nil, fmt.Errorf("llm: unknown api_format %q (supported: openai, anthropic)", apiFormat)
	}
}
