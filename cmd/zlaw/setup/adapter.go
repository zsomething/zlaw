package setup

import (
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// adapterScreenState holds state for adapter configuration screens.
type adapterScreenState struct {
	preset      string
	step        string // "select" or "secret"
	secretMode  string // "new" or "existing"
	secretName  string
	secretValue string
	cursor      int
	focused     int // 0=name, 1=value
	errMsg      string
}

// adapterPresets are the available adapter presets.
var adapterPresets = []string{"telegram", "slack", "none"}

// adapterEnvVars maps adapter preset names to required env vars.
var adapterEnvVars = map[string]string{
	"telegram": "TELEGRAM_BOT_TOKEN",
	"slack":    "SLACK_BOT_TOKEN",
}

// adapterInit initializes the adapter screen state.
func (m *Model) adapterInit() {
	m.adapter = &adapterScreenState{
		step:       "select",
		secretMode: "new",
		cursor:     0,
	}
}

// adapterView renders the adapter configuration screen.
func adapterView(m *Model) string {
	if m.adapter == nil {
		m.adapterInit()
	}

	var content strings.Builder

	if m.adapter.step == "select" {
		content.WriteString(Styles.Heading.Render("Configure Adapter — " + m.state.SelectedAgent))
		content.WriteString("\n")
		content.WriteString(Styles.Item.Render("Select adapter:"))
		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n")

		for i, name := range adapterPresets {
			prefix := "  "
			envVar := adapterEnvVars[name]
			if envVar == "" {
				envVar = "N/A"
			}
			if m.adapter.cursor == i {
				prefix = "> "
				content.WriteString(Styles.Selected.Render(prefix + itoa(i+1) + ". " + name + "  (" + envVar + ")"))
			} else {
				content.WriteString(Styles.Item.Render(prefix + itoa(i+1) + ". " + name + "  (" + envVar + ")"))
			}
			content.WriteString("\n")
		}

		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))

		return windowView("zlaw setup", content.String(), "[Enter] Select  [B] Back")
	}

	// Secret setup screen.
	presetName := m.adapter.preset
	envVar := adapterEnvVars[presetName]

	content.WriteString(Styles.Heading.Render("Adapter: " + presetName))
	content.WriteString("\n\n")
	content.WriteString(Styles.Item.Render("This adapter requires:"))
	content.WriteString("\n")
	content.WriteString(Styles.Item.Render("• bot_token — Env var: " + envVar))
	content.WriteString("\n\n")

	// Secret mode selection.
	content.WriteString(Styles.Item.Render("Secret:"))
	content.WriteString("\n")
	for i, opt := range []string{"Create new", "Use existing"} {
		prefix := "  "
		modeMatch := m.adapter.secretMode == strings.ToLower(strings.ReplaceAll(opt, " ", ""))
		if modeMatch {
			prefix = "> "
			var line string
			if m.adapter.secretMode == "new" {
				line = Styles.Selected.Render(prefix+opt) + "  [N]"
			} else {
				line = Styles.Selected.Render(prefix+opt) + "  [E]"
			}
			content.WriteString(line)
		} else {
			if i == 0 {
				content.WriteString(Styles.Item.Render(prefix + opt + "  [N]"))
			} else {
				content.WriteString(Styles.Item.Render(prefix + opt + "  [E]"))
			}
		}
		content.WriteString("\n")
	}

	content.WriteString("\n")

	if m.adapter.secretMode == "new" {
		// Secret name and value input.
		if m.adapter.focused == 0 {
			content.WriteString(Styles.Selected.Render("> Secret name: ") + Styles.Item.Render(m.adapter.secretName+"_"))
		} else {
			content.WriteString(Styles.Item.Render("  Secret name: ") + Styles.Item.Render(m.adapter.secretName))
		}
		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render("  > default: " + envVar))
		content.WriteString("\n\n")

		if m.adapter.focused == 1 {
			content.WriteString(Styles.Selected.Render("> Secret value: ") + Styles.Item.Render(strings.Repeat("*", intMax(0, len(m.adapter.secretValue)-3))+"***"))
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
				if m.adapter.cursor < len(secrets) && m.adapter.cursor == i {
					prefix = "> "
					content.WriteString(Styles.Selected.Render(prefix + name))
				} else {
					content.WriteString(Styles.Item.Render(prefix + name))
				}
				content.WriteString("\n")
			}
		}
	}

	if m.adapter.errMsg != "" {
		content.WriteString("\n")
		content.WriteString(Styles.StatusErr.Render("Error: " + m.adapter.errMsg))
	}

	content.WriteString("\n")
	content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))

	return windowView("zlaw setup", content.String(), "[C] Configure  [B] Back")
}

// updateAdapter handles keyboard events for the adapter configuration screen.
func updateAdapter(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.adapter == nil {
		m.adapterInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.adapter.step == "select" {
				if m.adapter.cursor > 0 {
					m.adapter.cursor--
				}
			} else if m.adapter.secretMode == "existing" {
				secrets := config.ListSecrets()
				if m.adapter.cursor > 0 && m.adapter.cursor < len(secrets) {
					m.adapter.cursor--
				}
			}
			return m, nil

		case "down", "j":
			if m.adapter.step == "select" {
				if m.adapter.cursor < len(adapterPresets)-1 {
					m.adapter.cursor++
				}
			} else if m.adapter.secretMode == "existing" {
				secrets := config.ListSecrets()
				if m.adapter.cursor < len(secrets)-1 {
					m.adapter.cursor++
				}
			}
			return m, nil

		case "enter":
			if m.adapter.step == "select" {
				if m.adapter.cursor < len(adapterPresets) {
					m.adapter.preset = adapterPresets[m.adapter.cursor]
					// Skip secret step for "none" preset.
					if m.adapter.preset == "none" {
						m2, cmd := adapterConfigure(m)
						return m2, cmd
					}
					m.adapter.step = "secret"
					m.adapter.secretName = adapterEnvVars[m.adapter.preset]
				}
			} else if m.adapter.secretMode == "existing" {
				secrets := config.ListSecrets()
				if m.adapter.cursor < len(secrets) {
					m.adapter.secretName = secrets[m.adapter.cursor]
					m2, cmd := adapterConfigure(m)
					return m2, cmd
				}
			}
			return m, nil

		case "tab":
			if m.adapter.step == "secret" && m.adapter.secretMode == "new" {
				m.adapter.focused = (m.adapter.focused + 1) % 2
			}
			return m, nil

		case "n", "N":
			if m.adapter.step == "secret" {
				m.adapter.secretMode = "new"
			}
			return m, nil

		case "e", "E":
			if m.adapter.step == "secret" {
				m.adapter.secretMode = "existing"
				m.adapter.cursor = 0
			}
			return m, nil

		case "c", "C":
			if m.adapter.step == "secret" {
				m2, cmd := adapterConfigure(m)
				return m2, cmd
			}
			return m, nil

		case "left", "h", "b", "B":
			if m.adapter.step == "secret" {
				m.adapter.step = "select"
				m.adapter.errMsg = ""
			} else {
				m.screen = ScreenMainMenu
				m.adapter = nil
			}
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		default:
			// Character input for secret fields.
			if m.adapter.step == "secret" && m.adapter.secretMode == "new" {
				if m.adapter.focused == 0 {
					// Secret name input.
					if isValidSecretNameChar(msg.Runes) {
						m.adapter.secretName += strings.ToUpper(string(msg.Runes))
					} else if msg.String() == "backspace" && len(m.adapter.secretName) > 0 {
						m.adapter.secretName = m.adapter.secretName[:len(m.adapter.secretName)-1]
					}
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// updateAdapterSecret is a no-op redirect to updateAdapter.
func updateAdapterSecret(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	return updateAdapter(m, msg)
}

// adapterConfigure saves the adapter configuration.
func adapterConfigure(m *Model) (tea.Model, tea.Cmd) {
	presetName := m.adapter.preset

	// Handle "none" preset - clear adapter config.
	if presetName == "none" {
		agentDir := m.state.Home + "/agents/" + m.state.SelectedAgent
		if err := config.WriteAdapterConfig(agentDir, nil); err != nil {
			m.adapter.errMsg = err.Error()
			return m, nil
		}

		state, err := LoadState()
		if err != nil {
			m.adapter.errMsg = err.Error()
			return m, nil
		}
		m.state = state
		m.screen = ScreenMainMenu
		m.adapter = nil
		return m, nil
	}

	envVar := adapterEnvVars[presetName]

	// Create secret if new mode.
	if m.adapter.secretMode == "new" {
		if m.adapter.secretName == "" {
			m.adapter.errMsg = "Secret name is required"
			return m, nil
		}
		if m.adapter.secretValue == "" {
			m.adapter.errMsg = "Secret value is required"
			return m, nil
		}
		if err := config.AddSecret(m.adapter.secretName, m.adapter.secretValue); err != nil {
			m.adapter.errMsg = err.Error()
			return m, nil
		}
	}

	// Write adapter config to agent.toml.
	agentDir := m.state.Home + "/agents/" + m.state.SelectedAgent
	adapterCfg := config.AdapterInstanceConfig{
		Backend: presetName,
		ClientConfig: map[string]any{
			"bot_token": "$" + m.adapter.secretName,
		},
	}

	if err := config.WriteAdapterConfig(agentDir, &adapterCfg); err != nil {
		m.adapter.errMsg = err.Error()
		return m, nil
	}

	// Add env var mapping to zlaw.toml.
	if err := config.SetAgentEnvVar(m.state.SelectedAgent, envVar, m.adapter.secretName); err != nil {
		m.adapter.errMsg = err.Error()
		return m, nil
	}

	// Reload state and go to menu.
	state, err := LoadState()
	if err != nil {
		m.adapter.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.screen = ScreenMainMenu
	m.adapter = nil
	return m, nil
}
