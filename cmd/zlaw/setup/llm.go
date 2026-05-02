package setup

import (
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
)

// llmScreenState holds state for LLM configuration screens.
type llmScreenState struct {
	preset      string
	step        string // "select" or "secret"
	secretMode  string // "new" or "existing"
	secretName  string
	secretValue string
	cursor      int
	focused     int // 0=name, 1=value
	dropdown    int // -1 = none
	errMsg      string
}

// presetEnvVars maps preset names to required env vars.
var presetEnvVars = map[string]string{
	"minimax":           "MINIMAX_API_KEY",
	"minimax-openai":    "MINIMAX_API_KEY",
	"minimax-cn":        "MINIMAX_API_KEY",
	"minimax-cn-openai": "MINIMAX_API_KEY",
	"anthropic":         "ANTHROPIC_API_KEY",
	"openai":            "OPENAI_API_KEY",
	"openrouter":        "OPENROUTER_API_KEY",
}

// llmInit initializes the LLM screen state.
func (m *Model) llmInit() {
	m.llm = &llmScreenState{
		step:       "select",
		secretMode: "new",
		cursor:     0,
		dropdown:   -1,
	}
}

// llmView renders the LLM configuration screen.
func llmView(m *Model) string {
	if m.llm == nil {
		m.llmInit()
	}

	var content strings.Builder

	if m.llm.step == "select" {
		content.WriteString(Styles.Heading.Render("Configure LLM — " + m.state.SelectedAgent))
		content.WriteString("\n\n")
		content.WriteString(Styles.Item.Render("Select LLM preset:"))
		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n")

		presets := llm.ListPresets()
		for i, name := range presets {
			prefix := "  "
			if m.llm.cursor == i {
				prefix = "▶ "
			}
			backend := getPresetBackend(name)
			envVar := presetEnvVars[name]
			if envVar == "" {
				envVar = "N/A"
			}
			line := Styles.Item.Render(prefix + name + "  —  " + backend + " (" + envVar + ")")
			if m.llm.cursor == i {
				line = Styles.Selected.Render(prefix + name + "  —  " + backend + " (" + envVar + ")")
			}
			content.WriteString(line)
			content.WriteString("\n")
		}

		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n")
		content.WriteString(Styles.Help.Render("[↑↓] Navigate  [Enter] Select  [←] Back"))
	} else {
		// Secret setup screen.
		presetName := m.llm.preset
		backend := getPresetBackend(presetName)
		envVar := presetEnvVars[presetName]

		content.WriteString(Styles.Heading.Render("LLM: " + presetName + " (" + backend + ")"))
		content.WriteString("\n\n")
		content.WriteString(Styles.Item.Render("This preset requires:"))
		content.WriteString("\n")
		content.WriteString(Styles.Item.Render("• api_key — Env var: " + envVar))
		content.WriteString("\n\n")

		// Secret mode selection.
		content.WriteString(Styles.Item.Render("Secret:"))
		content.WriteString("\n")
		for i, opt := range []string{"Create new", "Use existing"} {
			prefix := "  "
			if m.llm.secretMode == strings.ToLower(strings.ReplaceAll(opt, " ", "")) {
				prefix = "> "
				line := Styles.Selected.Render(prefix + opt)
				if m.llm.secretMode == "new" {
					line += "  [N]"
				} else {
					line += "  [E]"
				}
				content.WriteString(line)
				content.WriteString("\n")
			} else {
				if i == 0 {
					content.WriteString(Styles.Item.Render(prefix + opt + "  [N]"))
				} else {
					content.WriteString(Styles.Item.Render(prefix + opt + "  [E]"))
				}
				content.WriteString("\n")
			}
		}

		content.WriteString("\n")

		if m.llm.secretMode == "new" {
			// Secret name and value input.
			if m.llm.focused == 0 {
				content.WriteString(Styles.Selected.Render("> Secret name: ") + Styles.Item.Render(m.llm.secretName+"_"))
			} else {
				content.WriteString(Styles.Item.Render("  Secret name: ") + Styles.Item.Render(m.llm.secretName))
			}
			content.WriteString("\n")
			content.WriteString(Styles.ItemDim.Render("  > default: " + envVar))
			content.WriteString("\n\n")

			if m.llm.focused == 1 {
				content.WriteString(Styles.Selected.Render("> Secret value: ") + Styles.Item.Render("***"))
			} else {
				content.WriteString(Styles.Item.Render("  Secret value: [hidden]"))
			}
		} else {
			// Existing secrets list.
			secrets := config.ListSecrets()
			if len(secrets) == 0 {
				content.WriteString(Styles.ItemDim.Render("  No secrets found. Create one first."))
			} else {
				for i, name := range secrets {
					prefix := "  "
					if m.llm.cursor < len(secrets) && m.llm.cursor == i {
						prefix = "> "
					}
					if m.llm.cursor < len(secrets) && m.llm.cursor == i {
						content.WriteString(Styles.Selected.Render(prefix + name))
					} else {
						content.WriteString(Styles.Item.Render(prefix + name))
					}
					content.WriteString("\n")
				}
			}
		}

		if m.llm.errMsg != "" {
			content.WriteString("\n")
			content.WriteString(Styles.StatusErr.Render("Error: " + m.llm.errMsg))
		}

		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n")
		content.WriteString(Styles.Help.Render("[↑↓] Navigate  [Enter] Done  [←] Back"))
	}

	return windowView("zlaw setup", content.String(), "")
}

// getPresetBackend returns the backend for a preset.
func getPresetBackend(presetName string) string {
	p, err := llm.LookupPreset(presetName)
	if err != nil {
		return "unknown"
	}
	return p.Backend
}

// updateLLM handles keyboard events for the LLM configuration screen.
func updateLLM(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.llm == nil {
		m.llmInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.llm.step == "select" {
				if m.llm.cursor > 0 {
					m.llm.cursor--
				}
			} else if m.llm.secretMode == "existing" {
				secrets := config.ListSecrets()
				if m.llm.cursor > 0 && m.llm.cursor < len(secrets) {
					m.llm.cursor--
				}
			}
			return m, nil

		case "down", "j":
			if m.llm.step == "select" {
				presets := llm.ListPresets()
				if m.llm.cursor < len(presets)-1 {
					m.llm.cursor++
				}
			} else if m.llm.secretMode == "existing" {
				secrets := config.ListSecrets()
				if m.llm.cursor < len(secrets)-1 {
					m.llm.cursor++
				}
			}
			return m, nil

		case "enter":
			//nolint:staticcheck // tagged switch would require larger refactor
			if m.llm.step == "select" {
				presets := llm.ListPresets()
				if m.llm.cursor < len(presets) {
					m.llm.preset = presets[m.llm.cursor]
					m.llm.step = "secret"
					m.llm.secretName = presetEnvVars[m.llm.preset]
					if m.llm.secretName == "" {
						m.llm.secretName = "API_KEY"
					}
				}
			} else if m.llm.step == "secret" {
				if m.llm.secretMode == "existing" {
					secrets := config.ListSecrets()
					if m.llm.cursor < len(secrets) {
						m.llm.secretName = secrets[m.llm.cursor]
					}
				}
				// Configure with current values.
				m2, cmd := llmConfigure(m)
				return m2, cmd
			}
			return m, nil

		case "tab":
			if m.llm.step == "secret" && m.llm.secretMode == "new" {
				m.llm.focused = (m.llm.focused + 1) % 2
			}
			return m, nil

		case "left", "h":
			if m.llm.step == "secret" {
				m.llm.step = "select"
				m.llm.errMsg = ""
			} else {
				m.screen = ScreenMainMenu
				m.llm = nil
			}
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		default:
			// Character input for secret fields.
			if m.llm.step == "secret" && m.llm.secretMode == "new" {
				if m.llm.focused == 0 {
					// Secret name input.
					if isValidSecretNameChar(msg.Runes) {
						m.llm.secretName += strings.ToUpper(string(msg.Runes))
					} else if msg.String() == "backspace" && len(m.llm.secretName) > 0 {
						m.llm.secretName = m.llm.secretName[:len(m.llm.secretName)-1]
					}
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// updateLLMSecret is a no-op redirect to updateLLM.
func updateLLMSecret(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	return updateLLM(m, msg)
}

// llmConfigure saves the LLM configuration.
func llmConfigure(m *Model) (tea.Model, tea.Cmd) {
	presetName := m.llm.preset
	envVar := presetEnvVars[presetName]

	// Create secret if new mode.
	if m.llm.secretMode == "new" {
		if m.llm.secretName == "" {
			m.llm.errMsg = "Secret name is required"
			return m, nil
		}
		if m.llm.secretValue == "" {
			m.llm.errMsg = "Secret value is required"
			return m, nil
		}
		if err := config.AddSecret(m.llm.secretName, m.llm.secretValue); err != nil {
			m.llm.errMsg = err.Error()
			return m, nil
		}
	}

	// Get preset config.
	preset, err := llm.LookupPreset(presetName)
	if err != nil {
		m.llm.errMsg = "Preset not found: " + presetName
		return m, nil
	}

	// Write LLM config to agent.toml.
	agentDir := m.state.Home + "/agents/" + m.state.SelectedAgent
	llmConfig := config.LLMConfig{
		Backend: preset.Backend,
		ClientConfig: map[string]any{
			"base_url": preset.ClientConfig["base_url"],
			"api_key":  "$" + m.llm.secretName,
		},
		Model:       preset.DefaultModel,
		ModelConfig: preset.ModelConfig,
	}

	if err := config.WriteLLMConfig(agentDir, llmConfig); err != nil {
		m.llm.errMsg = err.Error()
		return m, nil
	}

	// Add env var mapping to zlaw.toml.
	if err := config.SetAgentEnvVar(m.state.SelectedAgent, envVar, m.llm.secretName); err != nil {
		m.llm.errMsg = err.Error()
		return m, nil
	}

	// Reload state and go to menu.
	state, err := LoadState()
	if err != nil {
		m.llm.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.screen = ScreenMainMenu
	m.llm = nil
	return m, nil
}

// isValidSecretNameChar checks if a rune is valid for a secret name.
func isValidSecretNameChar(runes []rune) bool {
	if len(runes) != 1 {
		return false
	}
	r := runes[0]
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
