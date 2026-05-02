package setup

import (
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/config"
)

// bootstrapState holds the state for the bootstrap screen.
type bootstrapState struct {
	configured bool
	cursor     int
}

// bootstrapInit initializes the state.
func (m *Model) bootstrapInit() {
	m.bootstrap = &bootstrapState{
		configured: m.state.IsConfigured(),
		cursor:     0,
	}
}

// bootstrapView renders the bootstrap screen.
func bootstrapView(m *Model) string {
	if m.bootstrap == nil {
		m.bootstrapInit()
	}

	var lines []string
	lines = append(lines, Styles.Heading.Render("Zlaw Home"))

	if m.bootstrap.configured {
		lines = append(lines, "")
		lines = append(lines, Styles.ItemDim.Render("  Location:"))
		lines = append(lines, Styles.Item.Render("  "+m.state.Home))
		lines = append(lines, "")
		lines = append(lines, Styles.StatusOK.Render("  ✓ Already configured"))
		lines = append(lines, "")
		lines = append(lines, Styles.Divider.Render(strings.Repeat("─", 40)))
		lines = append(lines, "")
		lines = append(lines, optionItem("Re-create zlaw home", m.bootstrap.cursor == 0, "[R]"))
		lines = append(lines, optionItem("Keep existing", m.bootstrap.cursor == 1, "[K]"))
		lines = append(lines, optionItem("Cancel", m.bootstrap.cursor == 2, "[N]"))
	} else {
		lines = append(lines, "")
		lines = append(lines, Styles.ItemDim.Render("  This will create:"))
		lines = append(lines, Styles.Item.Render("  • zlaw.toml"))
		lines = append(lines, Styles.Item.Render("  • secrets.toml"))
		lines = append(lines, Styles.Item.Render("  • agents/ directory"))
		lines = append(lines, "")
		lines = append(lines, Styles.Divider.Render(strings.Repeat("─", 40)))
		lines = append(lines, "")
		lines = append(lines, optionItem("Create", m.bootstrap.cursor == 0, "[Y]"))
		lines = append(lines, optionItem("Cancel", m.bootstrap.cursor == 1, "[N]"))
	}

	content := strings.Join(lines, "\n")
	return windowView("Bootstrap", content, "[←] Back  [Q] Quit")
}

// optionItem renders a selectable option.
func optionItem(label string, selected bool, shortcut string) string {
	prefix := "  "
	if selected {
		prefix = "▶ "
	}

	if selected {
		return Styles.Selected.Render(prefix+label) + "  " + Styles.Help.Render(shortcut)
	}
	return Styles.Item.Render(prefix+label) + "  " + Styles.ItemDim.Render(shortcut)
}

// updateBootstrap handles keyboard events for the bootstrap screen.
func updateBootstrap(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.bootstrap == nil {
		m.bootstrapInit()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.bootstrap.cursor > 0 {
				m.bootstrap.cursor--
			}
			return m, nil

		case "down", "j":
			maxCursor := 1
			if m.bootstrap.configured {
				maxCursor = 2
			}
			if m.bootstrap.cursor < maxCursor {
				m.bootstrap.cursor++
			}
			return m, nil

		case "left", "h":
			m.screen = ScreenMainMenu
			m.bootstrap = nil
			return m, nil

		case "enter", "l", "r", "R", "y", "Y":
			m2, cmd := bootstrapConfirm(m)
			return m2, cmd

		case "n", "N", "escape", "esc":
			m.screen = ScreenMainMenu
			m.bootstrap = nil
			return m, nil

		case "q", "Q", "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// bootstrapConfirm performs the action based on current cursor position.
func bootstrapConfirm(m *Model) (tea.Model, tea.Cmd) {
	if m.bootstrap.configured {
		if m.bootstrap.cursor == 0 {
			cfg := config.BootstrapConfig{Home: m.state.Home, Force: true}
			if err := cfg.CreateZlawHome(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		}
	} else {
		if m.bootstrap.cursor == 0 {
			cfg := config.BootstrapConfig{Home: m.state.Home}
			if err := cfg.CreateZlawHome(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		}
	}

	// Reload state and go to menu.
	state, err := LoadState()
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	m.state = state
	m.screen = ScreenMainMenu
	m.bootstrap = nil
	return m, nil
}
