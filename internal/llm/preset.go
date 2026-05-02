package llm

// LLMPreset is a named, well-known backend configuration.
// A preset defines default values; individual fields can be overridden in agent.toml.
// Values are copied inline at agent creation time — no runtime lookup needed.
type LLMPreset struct {
	// Name is the preset identifier (e.g., "minimax", "anthropic").
	Name string
	// Backend is the wire protocol ("openai" or "anthropic").
	Backend string
	// Config contains default values including base_url, model, max_tokens, etc.
	// These are copied inline into agent.toml at creation.
	Config map[string]any
}

// LookupPreset returns the preset for the given name, or an error if unknown.
func LookupPreset(name string) (LLMPreset, error) {
	preset, ok := presets[name]
	if !ok {
		return LLMPreset{}, nil
	}
	// Return a copy to prevent mutation of the original.
	return LLMPreset{
		Name:    preset.Name,
		Backend: preset.Backend,
		Config:  copyConfig(preset.Config),
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
