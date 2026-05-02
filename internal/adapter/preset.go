package adapter

// AdapterPreset is a named adapter configuration.
// Presets define default values; individual fields can be overridden in agent.toml.
type AdapterPreset struct {
	// Name is the preset identifier (e.g., "telegram", "slack").
	Name string
	// Backend is the adapter protocol (e.g., "telegram", "slack", "fizzy").
	Backend string
	// ClientConfig contains default adapter-specific values.
	ClientConfig map[string]any
}

// adapterPresets is the internal registry of named adapters.
var adapterPresets = map[string]AdapterPreset{
	"telegram": {
		Name:         "telegram",
		Backend:      "telegram",
		ClientConfig: map[string]any{
			"parse_mode": "Markdown",
		},
	},
	"slack": {
		Name:         "slack",
		Backend:      "slack",
		ClientConfig: map[string]any{
			"reaction": true,
		},
	},
}

// FindPreset returns the adapter preset for the given name, or an error if unknown.
func FindPreset(name string) (AdapterPreset, error) {
	preset, ok := adapterPresets[name]
	if !ok {
		return AdapterPreset{}, nil
	}
	return AdapterPreset{
		Name:    preset.Name,
		Backend: preset.Backend,
		ClientConfig: copyConfig(preset.ClientConfig),
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

// ListPresets returns all available adapter preset names.
func ListPresets() []string {
	names := make([]string, 0, len(adapterPresets))
	for name := range adapterPresets {
		names = append(names, name)
	}
	return names
}
