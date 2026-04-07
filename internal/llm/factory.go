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
func NewClientFromConfig(cfg config.LLMConfig, credentialsPath string, logger *slog.Logger) (Client, error) {
	if credentialsPath == "" {
		credentialsPath = auth.DefaultCredentialsPath()
	}

	src, err := auth.NewTokenSourceFromStore(credentialsPath, cfg.AuthProfile)
	if err != nil {
		return nil, fmt.Errorf("llm: load auth profile %q: %w", cfg.AuthProfile, err)
	}

	timeout := time.Duration(cfg.TimeoutSec) * time.Second

	switch cfg.Backend {
	case "minimax":
		return NewOpenAICompatClient(OpenAICompatConfig{
			BaseURL:     BaseURLMinimax,
			TokenSource: src,
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Timeout:     timeout,
			Logger:      logger,
		})
	case "openrouter":
		return NewOpenAICompatClient(OpenAICompatConfig{
			BaseURL:     BaseURLOpenRouter,
			TokenSource: src,
			Model:       cfg.Model,
			MaxTokens:   cfg.MaxTokens,
			Timeout:     timeout,
			Logger:      logger,
		})
	default:
		return nil, fmt.Errorf("llm: unknown backend %q (supported: minimax, openrouter)", cfg.Backend)
	}
}
