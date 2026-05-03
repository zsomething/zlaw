package setup

import (
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

// AppState holds the UI state.
type AppState struct {
	HomePath        string
	BootstrapStatus BootstrapStatus // not initialized, configured, or incomplete
	SelectedAgent   string
	Agents          []string
	LLMStatus       ItemState
	AdapterStatus   ItemState
	IdentityStatus  ItemState
	SoulStatus      ItemState
	SecretsCount    int
	SkillsCount     int
	EnvVarSet       bool // true if ZLAW_HOME env var is set
}

// LoadAppState reads the actual zlaw configuration.
func LoadAppState() *AppState {
	home := config.ZlawHome()
	zlawPath := filepath.Join(home, "zlaw.toml")

	// Determine bootstrap status
	var status BootstrapStatus
	_, err := os.Stat(zlawPath)
	if err == nil {
		// zlaw.toml exists - check if valid
		_, loadErr := config.LoadHubConfig(zlawPath)
		if loadErr != nil {
			status = BootstrapIncomplete
		} else {
			status = BootstrapReady
		}
	} else {
		// zlaw.toml doesn't exist - check if directory exists
		if os.IsNotExist(err) {
			// Directory may or may not exist - treat as not ready
			status = BootstrapNotReady
		} else {
			// Some other error - check if directory exists
			if _, dirErr := os.Stat(filepath.Dir(zlawPath)); dirErr == nil {
				status = BootstrapIncomplete
			} else {
				status = BootstrapNotReady
			}
		}
	}

	// Check if ZLAW_HOME env var is set
	envVarSet := os.Getenv("ZLAW_HOME") != ""

	// Load agents
	var agents []string
	if status == BootstrapReady {
		hub, err := config.LoadHubConfig(zlawPath)
		if err == nil {
			for _, a := range hub.Agents {
				agents = append(agents, a.ID)
			}
		}
	}

	// Count secrets
	secretsCount := 0
	secretsPath := filepath.Join(home, "secrets.toml")
	if _, err := os.Stat(secretsPath); err == nil {
		// TODO: count actual entries
		secretsCount = 1
	}

	return &AppState{
		HomePath:        home,
		BootstrapStatus: status,
		SelectedAgent:   "",
		Agents:          agents,
		LLMStatus:       StateMissing,
		AdapterStatus:   StateMissing,
		IdentityStatus:  StateMissing,
		SoulStatus:      StateMissing,
		SecretsCount:    secretsCount,
		SkillsCount:     0,
		EnvVarSet:       envVarSet,
	}
}

// DefaultAppState returns a stub state for testing (no actual config).
func DefaultAppState() *AppState {
	return &AppState{
		HomePath:        "/home/user/.config/zlaw",
		BootstrapStatus: BootstrapReady,
		SelectedAgent:   "assistant",
		Agents:          []string{"assistant", "gpt4"},
		LLMStatus:       StateConfigured,
		AdapterStatus:   StateConfigured,
		IdentityStatus:  StateConfigured,
		SoulStatus:      StateConfigured,
		SecretsCount:    2,
		SkillsCount:     3,
		EnvVarSet:       false,
	}
}
