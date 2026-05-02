package setup

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
	name       textinput.Model
	value      textinput.Model
	focused    int // 0=name, 1=value
	errMsg     string
	successMsg string
	quitting   bool
}

// secretsInit initializes the secrets screen state.
func (m *Model) secretsInit() {
	nameTi := textinput.New()
	nameTi.Prompt = ""
	nameTi.Placeholder = "SECRET_NAME"
	nameTi.CharLimit = 64
	nameTi.Width = 30

	valueTi := textinput.New()
	valueTi.Prompt = ""
	valueTi.Placeholder = "secret value"
	valueTi.CharLimit = 256
	valueTi.Width = 30
	valueTi.EchoMode = textinput.EchoNone

	m.secrets = &secretsScreenState{
		mode:   secretsModeList,
		cursor: 0,
		name:   nameTi,
		value:  valueTi,
	}
}

// secretsView renders the secrets management screen.
func secretsView(m *Model) string {
	if m.secrets == nil {
		m.secretsInit()
	}

	var content strings.Builder

	switch m.secrets.mode {
	case secretsModeList:
		content.WriteString(Styles.Item.Render("Manage secrets"))
		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n\n")

		secrets := config.ListSecrets()
		if len(secrets) == 0 {
			content.WriteString(Styles.ItemDim.Render("  No secrets defined."))
			content.WriteString("\n\n")
		} else {
			for i, name := range secrets {
				prefix := "  "
				if m.secrets.cursor == i {
					prefix = "▶ "
					content.WriteString(Styles.Selected.Render(prefix + name))
				} else {
					content.WriteString(Styles.Item.Render(prefix + name))
				}
				content.WriteString("\n")
			}
			content.WriteString("\n")
		}

		help := "[↑↓] Navigate  [Enter] Add/Delete  [Esc] Back"
		return windowView("zlaw setup", content.String(), help)

	case secretsModeAdd:
		content.WriteString(Styles.Item.Render("Add new secret"))
		content.WriteString("\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n\n")

		content.WriteString("Name:\n")
		if m.secrets.focused == 0 {
			content.WriteString(m.secrets.name.View())
		} else {
			content.WriteString(Styles.Item.Render("  " + m.secrets.name.Value()))
		}
		content.WriteString("\n\n")

		content.WriteString("Value:\n")
		if m.secrets.focused == 1 {
			content.WriteString(m.secrets.value.View())
		} else {
			val := m.secrets.value.Value()
			if val == "" {
				content.WriteString(Styles.ItemDim.Render("  [hidden]"))
			} else {
				content.WriteString(Styles.Item.Render("  [hidden]"))
			}
		}
		content.WriteString("\n")

		if m.secrets.errMsg != "" {
			content.WriteString("\n")
			content.WriteString(Styles.StatusErr.Render("Error: " + m.secrets.errMsg))
		}
		if m.secrets.successMsg != "" {
			content.WriteString("\n")
			content.WriteString(Styles.StatusOK.Render(m.secrets.successMsg))
		}

		help := "[Tab] Next field  [Enter] Save  [Esc] Cancel"
		return windowView("zlaw setup", content.String(), help)

	case secretsModeConfirm:
		secrets := config.ListSecrets()
		if m.secrets.cursor >= 0 && m.secrets.cursor < len(secrets) {
			name := secrets[m.secrets.cursor]
			content.WriteString(Styles.Item.Render("Delete secret?"))
			content.WriteString("\n")
			content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
			content.WriteString("\n\n")
			content.WriteString(Styles.Selected.Render("▶ " + name))
			content.WriteString("\n")

			if m.secrets.errMsg != "" {
				content.WriteString("\n")
				content.WriteString(Styles.StatusErr.Render("Error: " + m.secrets.errMsg))
			}

			help := "[Enter] Confirm  [Esc] Cancel"
			return windowView("zlaw setup", content.String(), help)
		}
	}

	return windowView("zlaw setup", "No secrets", "[Esc] Back")
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
			m.secrets.quitting = false
			switch m.secrets.mode {
			case secretsModeList:
				if m.secrets.cursor > 0 {
					m.secrets.cursor--
				}
			}
			return m, nil

		case "down", "j":
			m.secrets.quitting = false
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
				// Start add mode
				m.secrets.mode = secretsModeAdd
				m.secrets.name.SetValue("")
				m.secrets.value.SetValue("")
				m.secrets.focused = 0
				m.secrets.errMsg = ""
				m.secrets.successMsg = ""
				m.secrets.name.Focus()
				return m, nil

			case secretsModeAdd:
				// Validate
				if m.secrets.name.Value() == "" {
					m.secrets.errMsg = "Secret name is required"
					return m, nil
				}
				if m.secrets.value.Value() == "" {
					m.secrets.errMsg = "Secret value is required"
					return m, nil
				}
				// Save
				if err := config.AddSecret(m.secrets.name.Value(), m.secrets.value.Value()); err != nil {
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
				if m.secrets.cursor >= 0 && m.secrets.cursor < len(secrets) {
					name := secrets[m.secrets.cursor]
					if err := config.RemoveSecret(name); err != nil {
						m.secrets.errMsg = err.Error()
						return m, nil
					}
					m.secrets.mode = secretsModeList
					m.secrets.cursor = 0
					m.secrets.successMsg = "Secret deleted"
					m.secrets.errMsg = ""
					// Adjust cursor
					secrets = config.ListSecrets()
					if m.secrets.cursor >= len(secrets) && len(secrets) > 0 {
						m.secrets.cursor = len(secrets) - 1
					}
				}
				return m, nil
			}

		case "tab":
			if m.secrets.mode == secretsModeAdd {
				m.secrets.quitting = false
				if m.secrets.focused == 0 {
					m.secrets.name.Blur()
					m.secrets.value.Focus()
					m.secrets.focused = 1
				} else {
					m.secrets.value.Blur()
					m.secrets.name.Focus()
					m.secrets.focused = 0
				}
			}
			return m, nil

		case "escape", "esc", "left", "h":
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

		case "ctrl+c", "q", "Q":
			m.quit = true
			return m, tea.Quit

		case "delete", "backspace":
			if m.secrets.mode == secretsModeList {
				secrets := config.ListSecrets()
				if m.secrets.cursor < len(secrets) {
					m.secrets.mode = secretsModeConfirm
					m.secrets.errMsg = ""
				}
			}
			return m, nil
		}

		// Text input handling
		if m.secrets.mode == secretsModeAdd {
			var cmd tea.Cmd
			if m.secrets.focused == 0 {
				m.secrets.name, cmd = m.secrets.name.Update(msg)
				return m, cmd
			} else {
				m.secrets.value, cmd = m.secrets.value.Update(msg)
				return m, cmd
			}
		}
	}

	return m, nil
}
