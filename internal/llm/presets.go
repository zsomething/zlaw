package llm

// presets is the internal registry of named backends.
var presets = map[string]LLMPreset{
	// Minimax — global endpoint (Anthropic-compatible, recommended)
	"minimax": {
		Name:    "minimax",
		Backend: "anthropic",
		ClientConfig: map[string]any{
			"base_url": "https://api.minimax.io/anthropic",
		},
		ModelConfig: map[string]any{
			"max_tokens":     4096,
			"timeout_sec":    60,
			"prompt_caching": true,
		},
		DefaultModel: "MiniMax-Text-01",
	},
	// Minimax — global endpoint (OpenAI-compatible)
	"minimax-openai": {
		Name:    "minimax-openai",
		Backend: "openai",
		ClientConfig: map[string]any{
			"base_url": "https://api.minimax.io/v1",
		},
		ModelConfig: map[string]any{
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
		DefaultModel: "MiniMax-Text-01",
	},
	// Minimax — China endpoint (Anthropic-compatible, recommended)
	"minimax-cn": {
		Name:    "minimax-cn",
		Backend: "anthropic",
		ClientConfig: map[string]any{
			"base_url": "https://api.minimaxi.com/anthropic",
		},
		ModelConfig: map[string]any{
			"max_tokens":     4096,
			"timeout_sec":    60,
			"prompt_caching": true,
		},
		DefaultModel: "MiniMax-Text-01",
	},
	// Minimax — China endpoint (OpenAI-compatible)
	"minimax-cn-openai": {
		Name:    "minimax-cn-openai",
		Backend: "openai",
		ClientConfig: map[string]any{
			"base_url": "https://api.minimaxi.com/v1",
		},
		ModelConfig: map[string]any{
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
		DefaultModel: "MiniMax-Text-01",
	},
	// OpenRouter — aggregator
	"openrouter": {
		Name:    "openrouter",
		Backend: "openai",
		ClientConfig: map[string]any{
			"base_url": "https://openrouter.ai/api/v1",
		},
		ModelConfig: map[string]any{
			"max_tokens":  8192,
			"timeout_sec": 60,
		},
		DefaultModel: "anthropic/claude-3.5-sonnet",
	},
	// Anthropic — native Messages API
	"anthropic": {
		Name:    "anthropic",
		Backend: "anthropic",
		ClientConfig: map[string]any{
			"base_url": "https://api.anthropic.com",
		},
		ModelConfig: map[string]any{
			"max_tokens":     8192,
			"timeout_sec":   120,
			"prompt_caching": true,
		},
		DefaultModel: "claude-sonnet-4-20250514",
	},
	// OpenAI — direct OpenAI API
	"openai": {
		Name:    "openai",
		Backend: "openai",
		ClientConfig: map[string]any{
			"base_url": "https://api.openai.com/v1",
		},
		ModelConfig: map[string]any{
			"max_tokens":  4096,
			"timeout_sec": 60,
		},
		DefaultModel: "gpt-4o",
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
