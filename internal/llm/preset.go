package llm

import "fmt"

// APIFormat identifies the wire protocol used to communicate with a backend.
type APIFormat string

const (
	APIFormatOpenAI    APIFormat = "openai"
	APIFormatAnthropic APIFormat = "anthropic"
)

// BackendPreset is a named, well-known backend configuration.
// A preset defines default values; individual fields can be overridden in agent.toml.
type BackendPreset struct {
	BaseURL string
	APIFormat APIFormat
	// EmbeddingPreset is the preset name to use for embedding calls when this
	// backend is selected as the LLM backend. Empty means embeddings are not
	// auto-derivable from this backend.
	EmbeddingPreset string
}

// presets is the built-in registry of named backends.
var presets = map[string]BackendPreset{
	// Minimax — global endpoint (Anthropic-compatible, recommended)
	"minimax": {
		BaseURL:         "https://api.minimax.io/anthropic",
		APIFormat:       APIFormatAnthropic,
		EmbeddingPreset: "minimax-openai",
	},
	// Minimax — global endpoint (OpenAI-compatible)
	"minimax-openai": {
		BaseURL:         "https://api.minimax.io/v1",
		APIFormat:       APIFormatOpenAI,
		EmbeddingPreset: "minimax-openai",
	},
	// Minimax — China endpoint (Anthropic-compatible, recommended)
	"minimax-cn": {
		BaseURL:         "https://api.minimaxi.com/anthropic",
		APIFormat:       APIFormatAnthropic,
		EmbeddingPreset: "minimax-cn-openai",
	},
	// Minimax — China endpoint (OpenAI-compatible)
	"minimax-cn-openai": {
		BaseURL:         "https://api.minimaxi.com/v1",
		APIFormat:       APIFormatOpenAI,
		EmbeddingPreset: "minimax-cn-openai",
	},
	// OpenRouter — aggregator
	"openrouter": {
		BaseURL:         "https://openrouter.ai/api/v1",
		APIFormat:       APIFormatOpenAI,
		EmbeddingPreset: "openrouter",
	},
	// Anthropic — native Messages API (no embeddings endpoint)
	"anthropic": {
		BaseURL:   "https://api.anthropic.com",
		APIFormat: APIFormatAnthropic,
	},
}

// LookupPreset returns the preset for the given name, or an error if unknown.
func LookupPreset(name string) (BackendPreset, error) {
	p, ok := presets[name]
	if !ok {
		return BackendPreset{}, fmt.Errorf("llm: unknown backend preset %q", name)
	}
	return p, nil
}
