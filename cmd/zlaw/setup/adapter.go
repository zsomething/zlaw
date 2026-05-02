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

	lines := []string{
		Styles.Title.Render("zlaw setup"),
		"",
	}

	if m.adapter.step == "select" {
		lines = append(lines, Styles.Heading.Render("Configure Adapter — "+m.state.SelectedAgent))
		lines = append(lines, "")
		lines = append(lines, Styles.Item.Render("Select adapter:"))
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))

		for i, name := range adapterPresets {
			prefix := "  "
			if m.adapter.cursor == i {
				prefix = "> "
			}
			envVar := adapterEnvVars[name]
			if envVar == "" {
				envVar = "N/A"
			}
			line := Styles.Item.Render(prefix + itoa(i+1) + ". " + name + "  (" + envVar + ")")
			if m.adapter.cursor == i {
				line = Styles.Selected.Render(prefix + itoa(i+1) + ". " + name + "  (" + envVar + ")")
			}
			lines = append(lines, line)
		}

		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, Styles.Footer.Render("[Enter] Select  [B] Back"))
	} else {
		// Secret setup screen.
		presetName := m.adapter.preset
		envVar := adapterEnvVars[presetName]

		lines = append(lines, Styles.Heading.Render("Adapter: "+presetName))
		lines = append(lines, "")
		lines = append(lines, Styles.Item.Render("This adapter requires:"))
		lines = append(lines, Styles.Item.Render("• bot_token — Env var: "+envVar))
		lines = append(lines, "")

		// Secret mode selection.
		lines = append(lines, Styles.Item.Render("Secret:"))
		for i, opt := range []string{"Create new", "Use existing"} {
			prefix := "  "
			if m.adapter.secretMode == strings.ToLower(strings.ReplaceAll(opt, " ", "")) {
				prefix = "> "
				line := Styles.Selected.Render(prefix + opt)
				if m.adapter.secretMode == "new" {
					line += "  [N]"
				} else {
					line += "  [E]"
				}
				lines = append(lines, line)
			} else {
				if i == 0 {
					lines = append(lines, Styles.Item.Render(prefix+opt+"  [N]"))
				} else {
					lines = append(lines, Styles.Item.Render(prefix+opt+"  [E]"))
				}
			}
		}

		lines = append(lines, "")

		if m.adapter.secretMode == "new" {
			// Secret name and value input.
			if m.adapter.focused == 0 {
				lines = append(lines, Styles.Selected.Render("> Secret name: ")+Styles.Item.Render(m.adapter.secretName+"_"))
			} else {
				lines = append(lines, Styles.Item.Render("  Secret name: ")+Styles.Item.Render(m.adapter.secretName))
			}
			lines = append(lines, Styles.Dim.Render("  > default: "+envVar))

			lines = append(lines, "")
			if m.adapter.focused == 1 {
				lines = append(lines, Styles.Selected.Render("> Secret value: ")+Styles.Item.Render(strings.Repeat("*", intMax(0, len(m.adapter.secretValue)-3))+"***"))
			} else {
				lines = append(lines, Styles.Item.Render("  Secret value: [hidden]"))
			}
		} else {
			// Existing secrets list.
			secrets := config.ListSecrets()
			if len(secrets) == 0 {
				lines = append(lines, Styles.Dim.Render("  No secrets found. Create one first."))
			} else {
				for i, name := range secrets {
					prefix := "  "
					if m.adapter.cursor < len(secrets) && m.adapter.cursor == i {
						prefix = "> "
					}
					if m.adapter.cursor < len(secrets) && m.adapter.cursor == i {
						lines = append(lines, Styles.Selected.Render(prefix+name))
					} else {
						lines = append(lines, Styles.Item.Render(prefix+name))
					}
				}
			}
		}

		if m.adapter.errMsg != "" {
			lines = append(lines, "")
			lines = append(lines, Styles.StatusErr.Render("Error: "+m.adapter.errMsg))
		}

		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, Styles.Footer.Render("[C] Configure  [B] Back"))
	}

	return strings.Join(lines, "\n")
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
