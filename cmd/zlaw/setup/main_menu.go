package setup

import (
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) viewMainMenu() string {
	var b strings.Builder

	// Header
	b.WriteString(headerView("zlaw setup"))
	b.WriteString("\n\n")

	// Bootstrap section
	b.WriteString(Styles.SectionLabel.Render("BOOTSTRAP") + "\n")
	statusText, statusStyle := bootstrapStatusText(m.state.BootstrapStatus)
	b.WriteString(m.menuItem("Bootstrap Zlaw Home", statusText, statusStyle, m.cursor == 0) + "\n\n")

	// Agents section (show when bootstrapped)
	if m.state.BootstrapStatus == BootstrapReady {
		b.WriteString(Styles.SectionLabel.Render("AGENTS") + "\n")

		items := m.agentMenuItems()
		for i, item := range items {
			idx := i + 1
			itemText, itemStyle := itemStatus(item.state)
			b.WriteString(m.menuItem(item.label, itemText, itemStyle, m.cursor == idx) + "\n")
		}
		b.WriteString("\n")

		// Agent-scoped section (show when agent selected)
		if m.state.SelectedAgent != "" {
			b.WriteString(Styles.SectionLabel.Render("AGENT: "+m.state.SelectedAgent) + "\n")
			agentItems := m.agentScopedItems()
			// Calculate offset: bootstrap(1) + agents(agentItems count)
			offset := 1 + len(m.agentMenuItems())
			for i, item := range agentItems {
				idx := offset + i
				itemText, itemStyle := itemStatus(item.state)
				b.WriteString(m.menuItem(item.label, itemText, itemStyle, m.cursor == idx) + "\n")
			}
			b.WriteString("\n")
		}
	}

	// Global section
	b.WriteString(Styles.SectionLabel.Render("GLOBAL") + "\n")
	allItems := m.items()
	b.WriteString(m.menuItem("Manage Secrets", "", Styles.ItemDim, m.cursor == len(allItems)-2) + "\n")
	b.WriteString(m.menuItem("Summary", "", Styles.ItemDim, m.cursor == len(allItems)-1) + "\n\n")

	// Footer
	b.WriteString(divider())
	b.WriteString(footer("[↑↓] Navigate   [Enter] Select   [Q] Quit"))

	return b.String()
}

func (m *Model) menuItem(label string, status string, statusStyle lipgloss.Style, selected bool) string {
	text := label
	if status != "" {
		text += "  " + statusStyle.Render(status)
	}

	if selected {
		return Styles.ItemSelected.Render("▶ " + text)
	}
	return Styles.Item.Render("  " + text)
}

func (m *Model) updateMainMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	items := m.items()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(items)-1 {
				m.cursor++
			}
		case "enter":
			item := items[m.cursor]
			if item.screen != -1 {
				if item.disabled {
					return m, nil
				}
				m.pushScreen(item.screen)
			}
		}
	}
	return m, nil
}

// menuItemDef defines a menu item.
type menuItemDef struct {
	label    string
	screen   ScreenType
	state    ItemState
	disabled bool
}

// items returns all menu items based on current state.
func (m *Model) items() []menuItemDef {
	var items []menuItemDef

	// Bootstrap (always first)
	items = append(items, menuItemDef{
		label:  "Bootstrap Zlaw Home",
		screen: ScreenBootstrap,
		state:  StateView,
	})

	// Agents section (if bootstrapped)
	if m.state.BootstrapStatus == BootstrapReady {
		// Create Agent - always visible when bootstrapped
		items = append(items, menuItemDef{
			label:  "Create Agent",
			screen: ScreenAgentCreate,
			state:  StateView,
		})

		// Select Agent - only if agents exist
		if len(m.state.Agents) > 0 {
			// Show currently selected agent name
			selectLabel := "Select Agent"
			if m.state.SelectedAgent != "" {
				selectLabel = "Select Agent: " + m.state.SelectedAgent
			}
			items = append(items, menuItemDef{
				label:  selectLabel,
				screen: ScreenAgentConfig,
				state:  StateView,
			})
		}

		// Agent config items - only if agent selected
		if m.state.SelectedAgent != "" {
			items = append(items, menuItemDef{
				label:  "Configure LLM",
				screen: ScreenLLMConfig,
				state:  m.state.LLMStatus,
			})
			items = append(items, menuItemDef{
				label:  "Configure Adapter",
				screen: ScreenAdapterConfig,
				state:  m.state.AdapterStatus,
			})
			items = append(items, menuItemDef{
				label:  "Edit Identity",
				screen: ScreenIdentityEdit,
				state:  m.state.IdentityStatus,
			})
			items = append(items, menuItemDef{
				label:  "Edit Soul",
				screen: ScreenSoulEdit,
				state:  m.state.SoulStatus,
			})
			items = append(items, menuItemDef{
				label:  "Manage Skills",
				screen: ScreenSkills,
				state:  StateView,
			})
		}
	}

	// Global section (always visible)
	items = append(items, menuItemDef{
		label:  "Manage Secrets",
		screen: ScreenSecrets,
		state:  StateView,
	})
	items = append(items, menuItemDef{
		label:  "Summary",
		screen: ScreenSummary,
		state:  StateView,
	})

	return items
}

// agentMenuItems returns agent creation/selection items.
func (m *Model) agentMenuItems() []menuItemDef {
	if m.state.BootstrapStatus != BootstrapReady {
		return nil
	}
	var items []menuItemDef
	items = append(items, menuItemDef{
		label:  "Create Agent",
		screen: ScreenAgentCreate,
		state:  StateView,
	})
	if len(m.state.Agents) > 0 {
		selectLabel := "Select Agent"
		if m.state.SelectedAgent != "" {
			selectLabel = "Select Agent: " + m.state.SelectedAgent
		}
		items = append(items, menuItemDef{
			label:  selectLabel,
			screen: ScreenAgentConfig,
			state:  StateView,
		})
	}
	return items
}

// agentScopedItems returns agent-scoped configuration items.
func (m *Model) agentScopedItems() []menuItemDef {
	if m.state.SelectedAgent == "" {
		return nil
	}
	var items []menuItemDef
	items = append(items, menuItemDef{
		label:  "Configure LLM",
		screen: ScreenLLMConfig,
		state:  m.state.LLMStatus,
	})
	items = append(items, menuItemDef{
		label:  "Configure Adapter",
		screen: ScreenAdapterConfig,
		state:  m.state.AdapterStatus,
	})
	items = append(items, menuItemDef{
		label:  "Edit Identity",
		screen: ScreenIdentityEdit,
		state:  m.state.IdentityStatus,
	})
	items = append(items, menuItemDef{
		label:  "Edit Soul",
		screen: ScreenSoulEdit,
		state:  m.state.SoulStatus,
	})
	items = append(items, menuItemDef{
		label:  "Manage Skills",
		screen: ScreenSkills,
		state:  StateView,
	})
	return items
}

// itemStatus returns the status text and style for a menu item state.
func itemStatus(state ItemState) (string, lipgloss.Style) {
	switch state {
	case StateMissing:
		return "missing", Styles.StatusWarn
	case StateConfigured:
		return "configured", Styles.StatusOK
	case StateInvalid:
		return "invalid", Styles.StatusErr
	default:
		return "", Styles.ItemDim
	}
}

// Shared view helpers.
func headerView(title string) string {
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorAccent)).
			Render("▌"),
		Styles.Header.Render(" "+title),
	)
}

func divider() string {
	return strings.Repeat("─", FrameWidth) + "\n"
}

func footer(help string) string {
	return help
}