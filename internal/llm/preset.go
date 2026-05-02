package llm

// LLMPreset is a named, well-known backend configuration.
// A preset defines default values; copied inline into agent.toml at creation.
// No runtime lookup needed.
type LLMPreset struct {
	// Name is the preset identifier (e.g., "minimax", "anthropic").
	Name string
	// Backend is the wire protocol ("openai" or "anthropic").
	Backend string
	// ClientConfig contains default values for client construction.
	// Includes base_url, api_key (reference only, not secrets).
	ClientConfig map[string]any
	// ModelConfig contains provider-specific behavior defaults.
	// Includes max_tokens, timeout_sec, prompt_caching, etc.
	ModelConfig map[string]any
	// DefaultModel is the default model name.
	DefaultModel string
}

// LookupPreset returns the preset for the given name, or an error if unknown.
func LookupPreset(name string) (LLMPreset, error) {
	preset, ok := presets[name]
	if !ok {
		return LLMPreset{}, nil
	}
	// Return a copy to prevent mutation of the original.
	return LLMPreset{
		Name:         preset.Name,
		Backend:      preset.Backend,
		ClientConfig: copyConfig(preset.ClientConfig),
		ModelConfig:  copyConfig(preset.ModelConfig),
		DefaultModel: preset.DefaultModel,
	}, nil
}

// copyConfig returns a shallow copy of the config map.
func copyConfig(cfg map[string]any) map[string]any {
	if cfg == nil {
		return nil
	}
	result := make(map[string]any, len(cfg))
	for k, v := range cfg {
		result[k] = v
	}
	return result
}
