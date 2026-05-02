package setup

import (
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// secretsMode represents the current mode of the secrets screen.
type secretsMode int

const (
	secretsModeList secretsMode = iota
	secretsModeAdd
	secretsModeConfirm
)

// secretsScreenState holds state for the secrets management screen.
type secretsScreenState struct {
	mode       secretsMode
	cursor     int
	name       string
	value      string
	focused    int // 0=name, 1=value
	confirmIdx int // index of secret being deleted
	errMsg     string
	successMsg string
}

// secretsInit initializes the secrets screen state.
func (m *Model) secretsInit() {
	m.secrets = &secretsScreenState{
		mode:   secretsModeList,
		cursor: 0,
	}
}

// secretsView renders the secrets management screen.
func secretsView(m *Model) string {
	if m.secrets == nil {
		m.secretsInit()
	}

	var content strings.Builder
	var help string

	secrets := config.ListSecrets()

	switch m.secrets.mode {
	case secretsModeList:
		content.WriteString(Styles.Item.Render("Manage secrets") + "\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n\n")

		if len(secrets) == 0 {
			content.WriteString(Styles.ItemDim.Render("  No secrets defined.") + "\n\n")
		} else {
			content.WriteString(Styles.Item.Render("  Secret names:") + "\n")
			for i, name := range secrets {
				prefix := "  "
				if m.secrets.cursor == i {
					prefix = "▶ "
				}
				if m.secrets.cursor == i {
					content.WriteString(Styles.Selected.Render(prefix+name) + "\n")
				} else {
					content.WriteString(Styles.Item.Render(prefix+name) + "\n")
				}
			}
			content.WriteString("\n")
			content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n")
		}
		help = "[↑↓] Navigate  [Enter] Add/Delete  [←] Back"

	case secretsModeAdd:
		content.WriteString(Styles.Item.Render("Add new secret") + "\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n\n")

		if m.secrets.focused == 0 {
			content.WriteString(Styles.Selected.Render("> Name: ") + Styles.Item.Render(m.secrets.name+"_") + "\n")
		} else {
			content.WriteString(Styles.Item.Render("  Name: ") + Styles.Item.Render(m.secrets.name) + "\n")
		}

		content.WriteString("\n")

		if m.secrets.focused == 1 {
			if m.secrets.value == "" {
				content.WriteString(Styles.Selected.Render("> Value: ") + Styles.ItemDim.Render("[empty]") + "\n")
			} else {
				content.WriteString(Styles.Selected.Render("> Value: ") + Styles.Item.Render(strings.Repeat("*", 8)) + "\n")
			}
		} else {
			if m.secrets.value == "" {
				content.WriteString(Styles.Item.Render("  Value: ") + Styles.ItemDim.Render("[empty]") + "\n")
			} else {
				content.WriteString(Styles.Item.Render("  Value: ") + Styles.Item.Render(strings.Repeat("*", 8)) + "\n")
			}
		}

		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n")
		help = "[Tab] Switch field  [Enter] Save  [←] Cancel"

	case secretsModeConfirm:
		if m.secrets.confirmIdx >= 0 && m.secrets.confirmIdx < len(secrets) {
			name := secrets[m.secrets.confirmIdx]
			content.WriteString(Styles.Item.Render("Delete secret?") + "\n")
			content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n\n")
			content.WriteString(Styles.Selected.Render("  > "+name) + "\n\n")
			content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)) + "\n")
		}
		help = "[Enter] Confirm  [←] Cancel"
	}

	if m.secrets.errMsg != "" {
		content.WriteString("\n")
		content.WriteString(Styles.StatusErr.Render("Error: " + m.secrets.errMsg))
	}

	if m.secrets.successMsg != "" {
		content.WriteString("\n")
		content.WriteString(Styles.StatusOK.Render(m.secrets.successMsg))
	}

	return windowView("Secrets", content.String(), help)
}

// updateSecrets handles keyboard events for the secrets management screen.
func updateSecrets(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.secrets == nil {
		m.secretsInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			switch m.secrets.mode {
			case secretsModeList:
				secrets := config.ListSecrets()
				if m.secrets.cursor > 0 && m.secrets.cursor < len(secrets) {
					m.secrets.cursor--
				}
			}
			return m, nil

		case "down", "j":
			switch m.secrets.mode {
			case secretsModeList:
				secrets := config.ListSecrets()
				if m.secrets.cursor < len(secrets)-1 {
					m.secrets.cursor++
				}
			}
			return m, nil

		case "enter":
			switch m.secrets.mode {
			case secretsModeList:
				// Start add mode.
				m.secrets.mode = secretsModeAdd
				m.secrets.name = ""
				m.secrets.value = ""
				m.secrets.focused = 0
				m.secrets.errMsg = ""
				m.secrets.successMsg = ""
				return m, nil

			case secretsModeAdd:
				// Validate inputs.
				if m.secrets.name == "" {
					m.secrets.errMsg = "Secret name is required"
					return m, nil
				}
				if m.secrets.value == "" {
					m.secrets.errMsg = "Secret value is required"
					return m, nil
				}
				// Save secret.
				if err := config.AddSecret(m.secrets.name, m.secrets.value); err != nil {
					m.secrets.errMsg = err.Error()
					return m, nil
				}
				m.secrets.mode = secretsModeList
				m.secrets.cursor = 0
				m.secrets.successMsg = "Secret added"
				m.secrets.errMsg = ""
				return m, nil

			case secretsModeConfirm:
				secrets := config.ListSecrets()
				if m.secrets.confirmIdx >= 0 && m.secrets.confirmIdx < len(secrets) {
					name := secrets[m.secrets.confirmIdx]
					if err := config.RemoveSecret(name); err != nil {
						m.secrets.errMsg = err.Error()
						return m, nil
					}
					m.secrets.mode = secretsModeList
					m.secrets.cursor = 0
					m.secrets.successMsg = "Secret deleted"
					m.secrets.errMsg = ""
					// Adjust cursor if needed.
					secrets = config.ListSecrets()
					if m.secrets.cursor >= len(secrets) && len(secrets) > 0 {
						m.secrets.cursor = len(secrets) - 1
					}
				}
				return m, nil
			}

		case "tab":
			if m.secrets.mode == secretsModeAdd {
				m.secrets.focused = (m.secrets.focused + 1) % 2
			}
			return m, nil

		case "left", "h":
			switch m.secrets.mode {
			case secretsModeAdd:
				m.secrets.mode = secretsModeList
				m.secrets.errMsg = ""
			case secretsModeConfirm:
				m.secrets.mode = secretsModeList
				m.secrets.errMsg = ""
			default:
				m.screen = ScreenMainMenu
				m.secrets = nil
			}
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		default:
			// Character input for secret fields.
			if m.secrets.mode == secretsModeAdd {
				if m.secrets.focused == 0 {
					// Name input (uppercase letters, digits, underscore).
					if isValidSecretNameChar(msg.Runes) {
						m.secrets.name += strings.ToUpper(string(msg.Runes))
					} else if msg.String() == "backspace" && len(m.secrets.name) > 0 {
						m.secrets.name = m.secrets.name[:len(m.secrets.name)-1]
					}
				} else {
					// Value input (any printable character).
					if len(msg.Runes) == 1 && msg.Runes[0] >= 32 && msg.Runes[0] < 127 {
						m.secrets.value += string(msg.Runes)
					} else if msg.String() == "backspace" && len(m.secrets.value) > 0 {
						m.secrets.value = m.secrets.value[:len(m.secrets.value)-1]
					}
				}
			}
			return m, nil
		}
	}

	return m, nil
}
