package llm

// presets is the internal registry of named backends.
var presets = map[string]LLMPreset{
	// Minimax — global endpoint (Anthropic-compatible, recommended)
	"minimax": {
		Name:    "minimax",
		Backend: "anthropic",
		Config: map[string]any{
			"base_url":    "https://api.minimax.io/anthropic",
			"model":       "MiniMax-Text-01",
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
	},
	// Minimax — global endpoint (OpenAI-compatible)
	"minimax-openai": {
		Name:    "minimax-openai",
		Backend: "openai",
		Config: map[string]any{
			"base_url":    "https://api.minimax.io/v1",
			"model":       "MiniMax-Text-01",
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
	},
	// Minimax — China endpoint (Anthropic-compatible, recommended)
	"minimax-cn": {
		Name:    "minimax-cn",
		Backend: "anthropic",
		Config: map[string]any{
			"base_url":    "https://api.minimaxi.com/anthropic",
			"model":       "MiniMax-Text-01",
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
	},
	// Minimax — China endpoint (OpenAI-compatible)
	"minimax-cn-openai": {
		Name:    "minimax-cn-openai",
		Backend: "openai",
		Config: map[string]any{
			"base_url":    "https://api.minimaxi.com/v1",
			"model":       "MiniMax-Text-01",
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
	},
	// OpenRouter — aggregator
	"openrouter": {
		Name:    "openrouter",
		Backend: "openai",
		Config: map[string]any{
			"base_url":    "https://openrouter.ai/api/v1",
			"model":       "anthropic/claude-3.5-sonnet",
			"max_tokens":  8192,
			"timeout_sec": 60,
		},
	},
	// Anthropic — native Messages API
	"anthropic": {
		Name:    "anthropic",
		Backend: "anthropic",
		Config: map[string]any{
			"base_url":    "https://api.anthropic.com",
			"model":       "claude-sonnet-4-20250514",
			"max_tokens":  8192,
			"timeout_sec": 120,
		},
	},
	// OpenAI — direct OpenAI API
	"openai": {
		Name:    "openai",
		Backend: "openai",
		Config: map[string]any{
			"base_url":    "https://api.openai.com/v1",
			"model":       "gpt-4o",
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
	},
}

// ListPresets returns all available preset names.
func ListPresets() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	return names
}
