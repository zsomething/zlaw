package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// agentScreenState holds state for agent creation screen.
type agentScreenState struct {
	mode     string // "create"
	agentID  textinput.Model
	executor string // subprocess, systemd, docker
	target   string // local, ssh
	restart  string // always, on-failure, never
	cursor   int    // field cursor position
	focused  int    // which field is focused: 0=id, 1=executor, 2=target, 3=restart
	dropdown int    // -1 = no dropdown, 0+ = dropdown index
	errMsg   string
	quitting bool
}

// agentInit initializes the agent screen state.
func (m *Model) agentInit() {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "agent-id"
	ti.Focus()
	ti.CharLimit = 32
	ti.Width = 30

	m.agent = &agentScreenState{
		mode:     "create",
		agentID:  ti,
		executor: "subprocess",
		target:   "local",
		restart:  "on-failure",
		cursor:   0,
		dropdown: -1,
	}
}

// agentView renders the create agent screen.
func agentView(m *Model) string {
	if m.agent == nil {
		m.agentInit()
	}

	var lines []string

	// Title
	lines = append(lines, Styles.TitleBar.Render("  Create Agent"))
	lines = append(lines, "")

	// Form fields
	lines = append(lines, Styles.Item.Render("Agent ID:"))

	if m.agent.focused == 0 {
		lines = append(lines, m.agent.agentID.View())
	} else {
		lines = append(lines, Styles.ItemDim.Render("  "+m.agent.agentID.Value()))
	}
	lines = append(lines, Styles.ItemDim.Render("  lowercase, alphanumeric + dash"))
	lines = append(lines, "")

	// Dropdown fields
	fields := []struct {
		label   string
		options []string
		focus   int
	}{
		{"Executor", []string{"subprocess", "systemd", "docker"}, 1},
		{"Target", []string{"local", "ssh"}, 2},
		{"Restart policy", []string{"always", "on-failure", "never"}, 3},
	}

	currentValues := []string{m.agent.executor, m.agent.target, m.agent.restart}

	for i, field := range fields {
		lines = append(lines, Styles.Item.Render(field.label+":"))
		if m.agent.focused == field.focus && m.agent.dropdown < 0 {
			lines = append(lines, Styles.Selected.Render("▶ "+currentValues[i]))
		} else {
			lines = append(lines, Styles.Item.Render("  "+currentValues[i]))
		}
		lines = append(lines, "")
	}

	// Error message
	if m.agent.errMsg != "" {
		lines = append(lines, Styles.StatusErr.Render("Error: "+m.agent.errMsg))
		lines = append(lines, "")
	}

	// Help
	lines = append(lines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
	if m.agent.quitting {
		lines = append(lines, Styles.Help.Render("[Enter] Create  [Esc] Back"))
	} else {
		lines = append(lines, Styles.Help.Render("[↑↓] Navigate  [Enter] Select  [Esc] Back"))
	}

	return strings.Join(lines, "\n")
}

// updateAgent handles keyboard events for the agent creation screen.
func updateAgent(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		m.agentInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.agent.quitting = false
			if m.agent.focused > 0 {
				m.agent.focused--
				m.agent.dropdown = -1
			}
			return m, nil

		case "down", "j":
			m.agent.quitting = false
			if m.agent.focused < 3 {
				m.agent.focused++
				m.agent.dropdown = -1
			}
			return m, nil

		case "enter", "right", "l":
			if m.agent.dropdown >= 0 {
				// Select dropdown option
				options := dropdownOptions(m.agent.focused)
				if m.agent.dropdown < len(options) {
					setAgentField(m.agent, m.agent.focused, options[m.agent.dropdown])
				}
				m.agent.dropdown = -1
				return m, nil
			}

			if m.agent.focused == 0 {
				// Focus the text input
				m.agent.agentID.Focus()
				return m, nil
			}

			// If on last field, create agent
			if m.agent.focused == 3 {
				return agentCreate(m)
			}

			// Open dropdown for field
			m.agent.dropdown = 0
			return m, nil

		case "escape", "esc":
			m.screen = ScreenMainMenu
			m.agent = nil
			return m, nil

		case "left", "h":
			if m.agent.agentID.Focused() {
				m.agent.agentID.Blur()
			} else {
				m.screen = ScreenMainMenu
				m.agent = nil
			}
			return m, nil

		case "ctrl+c", "q", "Q":
			m.quit = true
			return m, tea.Quit

		case "tab":
			m.agent.quitting = false
			if m.agent.agentID.Focused() {
				m.agent.agentID.Blur()
			}
			m.agent.focused = (m.agent.focused + 1) % 4
			if m.agent.focused == 0 {
				m.agent.agentID.Focus()
			}
			return m, nil
		}

		// Let textinput handle character input when focused
		if m.agent.focused == 0 {
			var cmd tea.Cmd
			m.agent.agentID, cmd = m.agent.agentID.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// dropdownOptions returns options for each field.
func dropdownOptions(focus int) []string {
	switch focus {
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

// setAgentField sets a field value.
func setAgentField(s *agentScreenState, focus int, value string) {
	switch focus {
	case 1:
		s.executor = value
	case 2:
		s.target = value
	case 3:
		s.restart = value
	}
}

// agentCreate creates the agent.
func agentCreate(m *Model) (tea.Model, tea.Cmd) {
	if m.agent == nil {
		return m, nil
	}

	agentID := m.agent.agentID.Value()
	if agentID == "" {
		m.agent.errMsg = "Agent ID is required"
		return m, nil
	}

	cfg := config.SetupAgentConfig{
		ID:   agentID,
		Home: m.state.Home,
	}

	if err := cfg.CreateAgent(); err != nil {
		m.agent.errMsg = err.Error()
		return m, nil
	}

	// Reload state and go to menu
	state, err := LoadState()
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.state.SelectedAgent = agentID
	m.screen = ScreenMainMenu
	m.agent = nil
	return m, nil
}

// agentDelete deletes an agent. Reserved for future delete flow.
//
//nolint:unused // to be used when delete flow is implemented
func agentDelete(m *Model) (tea.Model, tea.Cmd) {
	if m.state.SelectedAgent == "" {
		return m, nil
	}

	agentDir := filepath.Join(m.state.Home, "agents", m.state.SelectedAgent)
	if err := os.RemoveAll(agentDir); err != nil {
		m.errMsg = err.Error()
		return m, nil
	}

	// Reload state
	state, err := LoadState()
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.screen = ScreenMainMenu
	m.agent = nil
	return m, nil
}

// selectAgentView renders the agent selector screen.
func selectAgentView(m *Model) string {
	var lines []string

	lines = append(lines, Styles.TitleBar.Render("  Select Agent"))
	lines = append(lines, "")
	lines = append(lines, Styles.Item.Render("Select an agent to configure:"))
	lines = append(lines, "")

	if len(m.state.Config.Agents) == 0 {
		lines = append(lines, Styles.ItemDim.Render("  No agents configured."))
	} else {
		for i, agent := range m.state.Config.Agents {
			prefix := "  "
			if m.cursor-1 == i { // -1 because menu has bootstrap item
				prefix = "▶ "
				lines = append(lines, Styles.Selected.Render(prefix+agent.ID))
			} else {
				lines = append(lines, Styles.Item.Render(prefix+agent.ID))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, Styles.ItemDim.Render(strings.Repeat("─", 32)))
	lines = append(lines, Styles.Help.Render("[↑↓] Navigate  [Enter] Select  [Esc] Back"))

	return strings.Join(lines, "\n")
}

// updateSelectAgent handles keyboard events for agent selection.
func updateSelectAgent(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 1 { // -1 for bootstrap menu item
				m.cursor--
			}
			return m, nil

		case "down", "j":
			agentCount := len(m.state.Config.Agents)
			if m.cursor < agentCount+1 { // +1 for bootstrap menu item
				m.cursor++
			}
			return m, nil

		case "enter", "right", "l":
			idx := m.cursor - 1 // -1 for bootstrap menu item
			if idx >= 0 && idx < len(m.state.Config.Agents) {
				m.state.SelectedAgent = m.state.Config.Agents[idx].ID
				m.screen = ScreenMainMenu
				return m, nil
			}
			return m, nil

		case "escape", "esc", "left", "h":
			m.screen = ScreenMainMenu
			return m, nil

		case "ctrl+c", "q", "Q":
			m.quit = true
			return m, tea.Quit
		}
	}

	return m, nil
}
