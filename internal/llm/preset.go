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
	BaseURL   string
	APIFormat APIFormat
}

// presets is the built-in registry of named backends.
var presets = map[string]BackendPreset{
	// Minimax — global endpoint
	"minimax": {
		BaseURL:   "https://api.minimax.io/v1",
		APIFormat: APIFormatOpenAI,
	},
	// Minimax — China endpoint
	"minimax-cn": {
		BaseURL:   "https://api.minimaxi.com/v1",
		APIFormat: APIFormatOpenAI,
	},
	// OpenRouter — aggregator
	"openrouter": {
		BaseURL:   "https://openrouter.ai/api/v1",
		APIFormat: APIFormatOpenAI,
	},
	// Anthropic — native Messages API
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
