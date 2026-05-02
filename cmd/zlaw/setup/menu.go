package setup

import (
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
	StateView
)

// MenuItem represents a single item in the menu.
type MenuItem struct {
	Label   string
	Status  ItemState
	Screen  Screen
	Visible bool
	Disable bool
}

// MenuItem represents a single item in the menu.
func buildMenuItems(m *Model) []MenuItem {
	items := []MenuItem{}

	// Bootstrap section - always visible
	bootstrapStatus := StateMissing
	if m.state.IsConfigured() {
		bootstrapStatus = StateConfigured
	}
	items = append(items, MenuItem{
		Label:   "Bootstrap Zlaw Home",
		Status:  bootstrapStatus,
		Screen:  ScreenBootstrap,
		Visible: true,
	})

	// Agents section
	if m.state.HasAgents() {
		// Agent selector info (non-selectable)
		agentCount := len(m.state.Config.Agents)
		selectedName := m.state.SelectedAgent
		if selectedName == "" && agentCount > 0 {
			selectedName = m.state.Config.Agents[0].ID
		}
		items = append(items, MenuItem{
			Label:   "Agent: " + selectedName + " (" + itoa(agentCount) + ")",
			Status:  StateView,
			Screen:  ScreenMainMenu,
			Visible: true,
			Disable: true,
		})

		// Agent config items (only when agent selected)
		if m.state.SelectedAgent != "" {
			// LLM configuration
			items = append(items, MenuItem{
				Label:   "Configure LLM",
				Status:  detectLLMState(m.state),
				Screen:  ScreenLLM,
				Visible: true,
			})

			// Adapter configuration
			items = append(items, MenuItem{
				Label:   "Configure adapter",
				Status:  detectAdapterState(m.state),
				Screen:  ScreenAdapter,
				Visible: true,
			})

			// Identity
			items = append(items, MenuItem{
				Label:   "Edit identity",
				Status:  detectIdentityState(m.state),
				Screen:  ScreenIdentity,
				Visible: true,
			})

			// Soul
			items = append(items, MenuItem{
				Label:   "Edit soul",
				Status:  detectSoulState(m.state),
				Screen:  ScreenSoul,
				Visible: true,
			})

			// Skills
			items = append(items, MenuItem{
				Label:   "Manage skills",
				Status:  StateView,
				Screen:  ScreenSkills,
				Visible: true,
			})
		}
	} else {
		// No agents message
		items = append(items, MenuItem{
			Label:   "No agents configured. Create one first.",
			Status:  StateMissing,
			Screen:  ScreenCreateAgent,
			Visible: true,
		})
	}

	// Global section
	items = append(items, MenuItem{
		Label:   "Manage secrets",
		Status:  StateView,
		Screen:  ScreenSecrets,
		Visible: true,
	})

	items = append(items, MenuItem{
		Label:   "Summary",
		Status:  StateView,
		Screen:  ScreenSummary,
		Visible: true,
	})

	return items
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

// menuItemSimpleView renders a single menu item (no sublabel, simpler)
func menuItemSimpleView(item MenuItem, selected bool) string {
	prefix := "  "
	if selected && !item.Disable {
		prefix = "▶ "
	}

	var label string
	if selected && !item.Disable {
		label = Styles.Selected.Render(prefix + item.Label)
	} else if item.Disable {
		label = Styles.ItemDim.Render(prefix + item.Label)
	} else {
		label = Styles.Item.Render(prefix + item.Label)
	}

	// Add status indicator for non-view items
	if item.Status != StateView {
		label += "  " + statusView(item.Status)
	}

	return label
}

func menuView(m *Model) string {
	items := getVisibleItems(buildMenuItems(m))
	if len(items) == 0 {
		return "No items"
	}

	// Group items by section
	var bootstrapItem, agentItems, globalItems []MenuItem

	for i, item := range items {
		if item.Disable {
			agentItems = append(agentItems, item)
		} else if i == 0 {
			bootstrapItem = append(bootstrapItem, item)
		} else if i >= len(items)-2 {
			globalItems = append(globalItems, item)
		} else {
			agentItems = append(agentItems, item)
		}
	}

	var lines []string

	// Bootstrap section
	for _, item := range bootstrapItem {
		lines = append(lines, menuItemSimpleView(item, m.cursor == 0))
	}

	if len(agentItems) > 0 {
		lines = append(lines, "")
		lines = append(lines, Styles.Heading.Render("Agents"))

		agentStart := 1
		for i, item := range agentItems {
			cursorIdx := agentStart + i
			lines = append(lines, menuItemSimpleView(item, m.cursor == cursorIdx))
		}
	}

	if len(globalItems) > 0 {
		lines = append(lines, "")
		lines = append(lines, Styles.Heading.Render("Global"))

		globalStart := len(items) - 2
		for i, item := range globalItems {
			cursorIdx := globalStart + i
			lines = append(lines, menuItemSimpleView(item, m.cursor == cursorIdx))
		}
	}

	content := strings.Join(lines, "\n")
	help := "[↑↓] Navigate  [Enter] Select  [Q] Quit"
	return windowView("zlaw setup", content, help)
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

		case "enter", "right", "l":
			items := getVisibleItems(buildMenuItems(m))
			if m.cursor < len(items) {
				selected := items[m.cursor]
				if selected.Disable {
					return m, nil
				}
				m.screen = selected.Screen
				return m, nil
			}
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
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

// agentHomeDir returns the agent's home directory path.
func agentHomeDir(home string, agent config.AgentEntry) string {
	if agent.Dir != "" {
		return agent.Dir
	}
	return filepath.Join(home, "agents", agent.ID)
}

// detectLLMState detects the LLM configuration state for the selected agent.
func detectLLMState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	_, ok := state.Config.FindAgent(state.SelectedAgent)
	if !ok {
		return StateMissing
	}
	return StateConfigured
}

// detectAdapterState detects the adapter configuration state.
func detectAdapterState(state *State) ItemState {
	return StateConfigured
}

// detectIdentityState checks if IDENTITY.md exists for the selected agent.
func detectIdentityState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	return StateConfigured
}

// detectSoulState checks if SOUL.md exists for the selected agent.
func detectSoulState(state *State) ItemState {
	if state.SelectedAgent == "" {
		return StateMissing
	}
	return StateConfigured
}

var _ = lipgloss.Style{} // compile-time check that lipgloss is used
