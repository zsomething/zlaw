package setup

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zsomething/zlaw/internal/config"
)

// ItemState represents the configuration state of a menu item.
type ItemState int

const (
	StateMissing ItemState = iota
	StateConfigured
	StateInvalid
	StateCount
	StateView
)

// MenuItem represents a single item in the menu.
type MenuItem struct {
	Label          string
	SubLabel       string
	State          ItemState
	Screen         Screen
	Visible        bool
	Disabled       bool
	DisabledReason string
}

// menuItemStateStyle returns the lipgloss style for an item state.
func menuItemStateStyle(s ItemState) lipgloss.Style {
	switch s {
	case StateConfigured:
		return Styles.StatusOK
	case StateMissing, StateInvalid:
		return Styles.StatusErr
	default:
		return Styles.Dim
	}
}

// menuItemStateText returns the text description for an item state.
func menuItemStateText(s ItemState) string {
	switch s {
	case StateConfigured:
		return "configured"
	case StateMissing:
		return "missing"
	case StateInvalid:
		return "invalid"
	case StateCount:
		return ""
	case StateView:
		return "view"
	default:
		return ""
	}
}

// buildMenuItems builds the list of menu items based on current state.
func buildMenuItems(m *Model) []MenuItem {
	items := []MenuItem{}

	// Bootstrap section - always visible
	bootstrapState := StateMissing
	if m.state.IsConfigured() {
		bootstrapState = StateConfigured
	}
	items = append(items, MenuItem{
		Label:    "Bootstrap Zlaw Home",
		SubLabel: m.state.Home,
		State:    bootstrapState,
		Screen:   ScreenBootstrap,
		Visible:  true,
	})

	// Agents section
	if m.state.HasAgents() {
		// Agent selector (shown as info, not selectable)
		agentCount := len(m.state.Config.Agents)
		selectedName := m.state.SelectedAgent
		if selectedName == "" && agentCount > 0 {
			selectedName = m.state.Config.Agents[0].ID
		}
		items = append(items, MenuItem{
			Label:    "Agent: " + selectedName,
			SubLabel: " (" + itoa(agentCount) + ")",
			State:    StateView,
			Screen:   ScreenMainMenu, // no-op for agent selector
			Visible:  true,
			Disabled: true,
		})

		// Agent sub-items (only when agent selected)
		if m.state.SelectedAgent != "" {
			// LLM configuration
			llmState := detectLLMState(m.state)
			llmModel := detectLLMModel(m.state)
			items = append(items, MenuItem{
				Label:    "Configure LLM",
				SubLabel: llmModel,
				State:    llmState,
				Screen:   ScreenLLM,
				Visible:  true,
			})

			// Adapter configuration
			adapterState := detectAdapterState(m.state)
			adapterName := detectAdapterName(m.state)
			items = append(items, MenuItem{
				Label:    "Configure adapter",
				SubLabel: adapterName,
				State:    adapterState,
				Screen:   ScreenAdapter,
				Visible:  true,
			})

			// Identity
			identityState := detectIdentityState(m.state)
			items = append(items, MenuItem{
				Label:   "Edit identity",
				State:   identityState,
				Screen:  ScreenIdentity,
				Visible: true,
			})

			// Soul
			soulState := detectSoulState(m.state)
			items = append(items, MenuItem{
				Label:   "Edit soul",
				State:   soulState,
				Screen:  ScreenSoul,
				Visible: true,
			})

			// Skills
			skillsCount := detectSkillsCount(m.state)
			items = append(items, MenuItem{
				Label:    "Manage skills",
				SubLabel: itoa(skillsCount) + " installed",
				State:    StateView,
				Screen:   ScreenSkills,
				Visible:  true,
			})
		}
	} else {
		// No agents message
		items = append(items, MenuItem{
			Label:    "No agents configured. Create one first.",
			State:    StateMissing,
			Screen:   ScreenCreateAgent,
			Visible:  true,
			Disabled: false,
		})
	}

	// Global section
	secretsCount := len(m.state.Secrets)
	items = append(items, MenuItem{
		Label:    "Manage secrets",
		SubLabel: itoa(secretsCount) + " secrets",
		State:    StateView,
		Screen:   ScreenSecrets,
		Visible:  true,
	})

	items = append(items, MenuItem{
		Label:    "Summary",
		SubLabel: "view",
		State:    StateView,
		Screen:   ScreenSummary,
		Visible:  true,
	})

	return items
}

// itoa converts an int to string without importing strconv.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// detectLLMState detects the LLM configuration state for the selected agent.
func detectLLMState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	agent, ok := state.Config.FindAgent(state.SelectedAgent)
	if !ok {
		return StateMissing
	}
	agentDir := agentDir(state.Home, agent)
	// Check if agent.toml has LLM config
	if _, err := os.Stat(filepath.Join(agentDir, "agent.toml")); os.IsNotExist(err) {
		return StateMissing
	}
	return StateConfigured
}

// agentDir returns the agent's directory path.
func agentDir(home string, agent config.AgentEntry) string {
	if agent.Dir != "" {
		return agent.Dir
	}
	return filepath.Join(home, "agents", agent.ID)
}

// detectLLMModel returns the configured LLM model name.
func detectLLMModel(state *State) string {
	return "minimax" // TODO: implement actual detection
}

// detectAdapterState detects the adapter configuration state.
func detectAdapterState(state *State) ItemState {
	return StateConfigured // TODO: implement actual detection
}

// detectAdapterName returns the configured adapter name.
func detectAdapterName(state *State) string {
	return "telegram" // TODO: implement actual detection
}

// detectIdentityState checks if IDENTITY.md exists for the selected agent.
func detectIdentityState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	return StateConfigured // TODO: implement actual file check
}

// detectSoulState checks if SOUL.md exists for the selected agent.
func detectSoulState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	return StateConfigured // TODO: implement actual file check
}

// detectSkillsCount returns the number of installed skills.
func detectSkillsCount(state *State) int {
	return 0 // TODO: implement actual count
}

// getVisibleItems returns only visible menu items.
func getVisibleItems(items []MenuItem) []MenuItem {
	visible := []MenuItem{}
	for _, item := range items {
		if item.Visible {
			visible = append(visible, item)
		}
	}
	return visible
}

// updateMenu handles keyboard events for the main menu screen.
func updateMenu(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			items := getVisibleItems(buildMenuItems(m))
			if m.cursor < len(items)-1 {
				m.cursor++
			}
			return m, nil

		case "enter", "right":
			items := getVisibleItems(buildMenuItems(m))
			if m.cursor < len(items) {
				selected := items[m.cursor]
				if selected.Disabled {
					return m, nil
				}
				// Navigate to the selected screen
				m.screen = selected.Screen
				return m, nil
			}
			return m, nil

		case "q", "Q":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// menuView renders the main menu view.
// menuItems returns a slice of all menu items.
// Structure: [Bootstrap, ...Agent items..., Manage secrets, Summary]
func menuItems(m *Model) []MenuItem {
	items := getVisibleItems(buildMenuItems(m))
	return items
}

func menuView(m *Model) string {
	items := menuItems(m)
	if len(items) == 0 {
		return "No items"
	}

	var lines []string

	lines = append(lines, Styles.Title.Render("zlaw setup"))
	lines = append(lines, "")

	// Bootstrap section (always first item)
	lines = append(lines, Styles.Heading.Render("Bootstrap"))
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
	lines = append(lines, menuItemView(items[0], m.cursor == 0))

	// Agents section
	lines = append(lines, "")
	lines = append(lines, Styles.Heading.Render("Agents"))
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))

	agentStart := 1
	agentEnd := len(items) - 2 // Exclude Global items
	if agentEnd <= agentStart {
		// No agents section items, show message if configured
		if len(items) > 1 && items[1].Label == "No agents configured. Create one first." {
			lines = append(lines, menuItemView(items[1], m.cursor == 1))
		}
	} else {
		for idx := agentStart; idx < agentEnd; idx++ {
			item := items[idx]
			// Render agent selector as dimmed info
			if item.Disabled && item.State == StateView && strings.HasPrefix(item.Label, "Agent: ") {
				lines = append(lines, Styles.Dim.Render(item.Label+item.SubLabel))
				lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
				continue
			}
			line := menuItemView(item, m.cursor == idx)
			lines = append(lines, line)
		}
	}

	// Global section (last 2 items)
	lines = append(lines, "")
	lines = append(lines, Styles.Heading.Render("Global"))
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))

	globalStart := len(items) - 2
	for idx := globalStart; idx < len(items); idx++ {
		item := items[idx]
		line := menuItemView(item, m.cursor == idx)
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
	lines = append(lines, Styles.Footer.Render("[Q] Quit  [↑↓] Navigate  [Enter] Select"))

	return strings.Join(lines, "\n")
}

// menuItemView renders a single menu item.
func menuItemView(item MenuItem, selected bool) string {
	var label string
	if selected && !item.Disabled {
		label = Styles.Selected.Render("> " + item.Label)
	} else {
		label = Styles.Item.Render(item.Label)
	}

	if item.SubLabel != "" {
		label += " " + Styles.ItemHelp.Render(item.SubLabel)
	}

	if item.State != StateView {
		stateText := menuItemStateText(item.State)
		label += "  " + menuItemStateStyle(item.State).Render(stateText)
	}

	return label
}
