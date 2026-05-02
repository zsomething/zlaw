package setup

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// agentScreenState holds state for agent creation/deletion screens.
type agentScreenState struct {
	mode     string // "create" or "delete"
	agentID  string
	executor string // subprocess, systemd, docker
	target   string // local, ssh
	restart  string // always, on-failure, never
	cursor   int    // field cursor position
	focused  int    // which field is focused: 0=id, 1=executor, 2=target, 3=restart
	dropdown int    // -1 = no dropdown, 0+ = dropdown index
	errMsg   string
}

var (
	agentIDRegex  = regexp.MustCompile(`^[a-z0-9-]+$`)
	agentIDPrompt = "lowercase, alphanumeric + dash"
)

// agentInit initializes the agent screen state.
func (m *Model) agentInit(mode string) {
	m.agent = &agentScreenState{
		mode:     mode,
		executor: "subprocess",
		target:   "local",
		restart:  "on-failure",
		cursor:   0,
		dropdown: -1,
	}
	if mode == "delete" && m.state.SelectedAgent != "" {
		m.agent.agentID = m.state.SelectedAgent
	}
}

// agentView renders the create/delete agent screen.
func agentView(m *Model) string {
	if m.agent == nil {
		mode := "create"
		if m.screen == ScreenDeleteAgent {
			mode = "delete"
		}
		m.agentInit(mode)
	}

	var content strings.Builder

	if m.agent.mode == "create" {
		content.WriteString(Styles.Heading.Render("Create Agent"))
		content.WriteString("\n\n")
		content.WriteString(agentFieldView(m, "Agent ID:", m.agent.agentID, agentIDPrompt, 0))
		content.WriteString("\n")
		content.WriteString(agentDropdownView(m, "Executor:", m.agent.executor, []string{"subprocess", "systemd", "docker"}, 1))
		content.WriteString("\n")
		content.WriteString(agentDropdownView(m, "Target:", m.agent.target, []string{"local", "ssh"}, 2))
		content.WriteString("\n")
		content.WriteString(agentDropdownView(m, "Restart:", m.agent.restart, []string{"always", "on-failure", "never"}, 3))
	} else {
		content.WriteString(Styles.Heading.Render("Delete Agent?"))
		content.WriteString("\n\n")
		content.WriteString(Styles.Item.Render("Agent: " + m.agent.agentID))
		content.WriteString("\n\n")
		content.WriteString(Styles.ItemDim.Render(strings.Repeat("─", 32)))
		content.WriteString("\n")
		content.WriteString(agentOption(m, "Delete", 'D', 0))
		content.WriteString("\n")
		content.WriteString(agentOption(m, "Cancel", 'N', 1))
	}

	if m.agent.errMsg != "" {
		content.WriteString("\n")
		content.WriteString(Styles.StatusErr.Render("Error: " + m.agent.errMsg))
	}

	var help string
	if m.agent.mode == "create" {
		help = "[C] Create  [B] Back  [Tab] Next field"
	} else {
		help = "[D] Delete  [N] Cancel  [B] Back"
	}

	return windowView("Setup", content.String(), help)
}

// agentFieldView renders a text input field.
func agentFieldView(m *Model, label, value, hint string, fieldIdx int) string {
	focused := m.agent.focused == fieldIdx && m.agent.dropdown < 0
	cursor := " "
	if focused {
		cursor = ">"
	}

	if focused {
		return Styles.Selected.Render(cursor+" "+label) + " " +
			Styles.Item.Render(value) + Styles.ItemDim.Render("_"+strings.Repeat(" ", intMax(0, 20-len(value)))) + "\n" +
			Styles.ItemDim.Render("  > "+hint)
	}
	return Styles.Item.Render(cursor+" "+label) + " " +
		Styles.Item.Render(value) + Styles.ItemDim.Render(strings.Repeat(" ", intMax(0, 20-len(value))))
}

// agentDropdownView renders a dropdown field.
func agentDropdownView(m *Model, label, value string, options []string, fieldIdx int) string {
	focused := m.agent.focused == fieldIdx && m.agent.dropdown < 0
	cursor := " "
	if focused {
		cursor = ">"
	}

	if m.agent.dropdown == fieldIdx {
		// Show options
		lines := []string{}
		lines = append(lines, Styles.Selected.Render("> "+label))
		for i, opt := range options {
			if i == m.agent.cursor && i < len(options) {
				lines = append(lines, Styles.Selected.Render("  > "+opt))
			} else {
				lines = append(lines, Styles.Item.Render("    "+opt))
			}
		}
		return strings.Join(lines, "\n")
	}

	if focused {
		return Styles.Selected.Render(cursor+" "+label) + " " +
			Styles.Item.Render("["+value+" ▼]")
	}
	return Styles.Item.Render(cursor+" "+label) + " " +
		Styles.Item.Render("["+value+" ▼]")
}

// agentOption renders a single option.
func agentOption(m *Model, label string, key rune, idx int) string {
	if m.agent.cursor == idx {
		return Styles.Selected.Render("> " + label + "  [" + string(key) + "]")
	}
	return Styles.Item.Render(label + "  [" + string(key) + "]")
}

// intMax returns the maximum of two integers.
func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// updateCreateAgent handles keyboard events for the create agent screen.
func updateCreateAgent(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.agentInit("create")
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			// Move to next field or close dropdown.
			if m.agent.dropdown >= 0 {
				// Close dropdown and select current option.
				options := agentOptionsForField(m.agent.focused)
				if m.agent.cursor < len(options) {
					*agentFieldPtr(m.agent, m.agent.focused) = options[m.agent.cursor]
				}
				m.agent.dropdown = -1
				m.agent.cursor = 0
			} else {
				m.agent.focused = (m.agent.focused + 1) % 4
			}
			return m, nil

		case "enter":
			if m.agent.dropdown >= 0 {
				options := agentOptionsForField(m.agent.focused)
				if m.agent.cursor < len(options) {
					*agentFieldPtr(m.agent, m.agent.focused) = options[m.agent.cursor]
				}
				m.agent.dropdown = -1
				m.agent.cursor = 0
			} else {
				m2, cmd := agentCreate(m)
				return m2, cmd
			}
			return m, nil

		case "up", "k":
			if m.agent.dropdown >= 0 {
				if m.agent.cursor > 0 {
					m.agent.cursor--
				}
			}
			return m, nil

		case "down", "j":
			if m.agent.dropdown >= 0 {
				options := agentOptionsForField(m.agent.focused)
				if m.agent.cursor < len(options)-1 {
					m.agent.cursor++
				}
			}
			return m, nil

		case "escape", "esc":
			if m.agent.dropdown >= 0 {
				m.agent.dropdown = -1
				m.agent.cursor = 0
			}
			return m, nil

		case "left", "h":
			// Back to menu.
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "b", "B":
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "c", "C":
			m2, cmd := agentCreate(m)
			return m2, cmd

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		default:
			// Character input for ID field.
			if m.agent.focused == 0 && m.agent.dropdown < 0 {
				if isValidAgentIDChar(msg.Runes) {
					m.agent.agentID += strings.ToLower(string(msg.Runes))
				} else if msg.String() == "backspace" && len(m.agent.agentID) > 0 {
					m.agent.agentID = m.agent.agentID[:len(m.agent.agentID)-1]
				}
			}
			return m, nil
		}
	}

	return m, nil
}

// updateDeleteAgent handles keyboard events for the delete agent screen.
func updateDeleteAgent(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.agentInit("delete")
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.agent.cursor > 0 {
				m.agent.cursor--
			}
			return m, nil

		case "down", "j":
			if m.agent.cursor < 1 {
				m.agent.cursor++
			}
			return m, nil

		case "enter", "right", "l":
			if m.agent.cursor == 0 {
				m2, cmd := agentDelete(m)
				return m2, cmd
			}
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "d", "D":
			m2, cmd := agentDelete(m)
			return m2, cmd

		case "n", "N":
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "left", "h", "b", "B":
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// agentCreate creates the agent.
func agentCreate(m *Model) (tea.Model, tea.Cmd) {
	// Validate ID.
	if !agentIDRegex.MatchString(m.agent.agentID) || len(m.agent.agentID) < 2 {
		m.agent.errMsg = "Agent ID must be 2+ chars: lowercase, alphanumeric, dash only"
		return m, nil
	}

	// Check for duplicates.
	_, exists := m.state.Config.FindAgent(m.agent.agentID)
	if exists {
		m.agent.errMsg = "Agent " + m.agent.agentID + " already exists"
		return m, nil
	}

	// Create agent using shared config.
	cfg := config.SetupAgentConfig{ID: m.agent.agentID}
	if err := cfg.CreateAgent(); err != nil {
		m.agent.errMsg = err.Error()
		return m, nil
	}

	// Add to zlaw.toml.
	entry := config.AgentEntry{
		ID:            m.agent.agentID,
		Executor:      m.agent.executor,
		Target:        m.agent.target,
		RestartPolicy: config.RestartPolicy(m.agent.restart),
	}
	if err := m.state.Config.AddAgent(entry); err != nil {
		m.agent.errMsg = "Failed to add agent to config: " + err.Error()
		return m, nil
	}

	// Reload state and go to menu.
	state, err := LoadState()
	if err != nil {
		m.agent.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.state.SelectedAgent = m.agent.agentID
	m.screen = ScreenMainMenu
	m.agent = nil
	return m, nil
}

// agentDelete deletes the agent.
func agentDelete(m *Model) (tea.Model, tea.Cmd) {
	agentID := m.agent.agentID

	// Remove from zlaw.toml.
	if err := m.state.Config.RemoveAgent(agentID); err != nil {
		m.agent.errMsg = "Failed to remove agent from config: " + err.Error()
		return m, nil
	}

	// Remove agent directory.
	agentDir := filepath.Join(m.state.Home, "agents", agentID)
	if err := os.RemoveAll(agentDir); err != nil {
		m.agent.errMsg = "Failed to remove agent directory: " + err.Error()
		return m, nil
	}

	// Clear selection if this was the selected agent.
	if m.state.SelectedAgent == agentID {
		m.state.SelectedAgent = ""
	}

	// Reload state and go to menu.
	state, err := LoadState()
	if err != nil {
		m.agent.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.screen = ScreenMainMenu
	m.agent = nil
	return m, nil
}

// agentOptionsForField returns the options for a field index.
func agentOptionsForField(fieldIdx int) []string {
	switch fieldIdx {
	case 1:
		return []string{"subprocess", "systemd", "docker"}
	case 2:
		return []string{"local", "ssh"}
	case 3:
		return []string{"always", "on-failure", "never"}
	default:
		return nil
	}
}

// agentFieldPtr returns a pointer to the field value.
func agentFieldPtr(a *agentScreenState, fieldIdx int) *string {
	switch fieldIdx {
	case 0:
		return &a.agentID
	case 1:
		return &a.executor
	case 2:
		return &a.target
	case 3:
		return &a.restart
	default:
		return nil
	}
}

// isValidAgentIDChar checks if a rune is valid for an agent ID.
func isValidAgentIDChar(runes []rune) bool {
	if len(runes) != 1 {
		return false
	}
	r := runes[0]
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
}
