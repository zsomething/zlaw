package setup

import (
	"os"
	"path/filepath"
	"strings"

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

	// Load agents and detect per-agent status
	var agents []string
	var selectedAgent string
	var llmStatus, adapterStatus, identityStatus, soulStatus ItemState
	var skillsCount int
	if status == BootstrapReady {
		hub, err := config.LoadHubConfig(zlawPath)
		if err == nil {
			for _, a := range hub.Agents {
				agents = append(agents, a.ID)
			}
			// Set first agent as default selected
			if len(agents) > 0 {
				selectedAgent = agents[0]

				// Load per-agent status
				agentDir := filepath.Join(home, "agents", selectedAgent)
				llmStatus = detectLLMStatus(agentDir)
				adapterStatus = detectAdapterStatus(agentDir)
				identityStatus = detectIdentityStatus(agentDir)
				soulStatus = detectSoulStatus(agentDir)
				skillsCount = detectSkillsCount(agentDir)
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
		SelectedAgent:   selectedAgent,
		Agents:          agents,
		LLMStatus:       llmStatus,
		AdapterStatus:   adapterStatus,
		IdentityStatus:  identityStatus,
		SoulStatus:      soulStatus,
		SecretsCount:    secretsCount,
		SkillsCount:     skillsCount,
		EnvVarSet:       envVarSet,
	}
}

// detectLLMStatus checks if agent.toml has [llm] section.
func detectLLMStatus(agentDir string) ItemState {
	cfgPath := filepath.Join(agentDir, "agent.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		return StateMissing
	}
	cfg, err := config.LoadAgentConfigFile(cfgPath)
	if err != nil || cfg.LLM.Backend == "" {
		return StateMissing
	}
	return StateConfigured
}

// detectAdapterStatus checks if agent.toml has [adapter] section.
func detectAdapterStatus(agentDir string) ItemState {
	cfgPath := filepath.Join(agentDir, "agent.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		return StateMissing
	}
	cfg, err := config.LoadAgentConfigFile(cfgPath)
	if err != nil || len(cfg.Adapter) == 0 {
		return StateMissing
	}
	return StateConfigured
}

// detectIdentityStatus checks if IDENTITY.md exists.
func detectIdentityStatus(agentDir string) ItemState {
	identityPath := filepath.Join(agentDir, "IDENTITY.md")
	if _, err := os.Stat(identityPath); err == nil {
		return StateConfigured
	}
	return StateMissing
}

// detectSoulStatus checks if SOUL.md exists.
func detectSoulStatus(agentDir string) ItemState {
	soulPath := filepath.Join(agentDir, "SOUL.md")
	if _, err := os.Stat(soulPath); err == nil {
		return StateConfigured
	}
	return StateMissing
}

// detectSkillsCount counts .md files in skills/ directory.
func detectSkillsCount(agentDir string) int {
	skillsDir := filepath.Join(agentDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			count++
		}
	}
	return count
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
