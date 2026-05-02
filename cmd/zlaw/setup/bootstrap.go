package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"

	"github.com/zsomething/zlaw/internal/secrets"
)

// bootstrapState holds the state for the bootstrap screen.
type bootstrapState struct {
	configured bool
	cursor     int // 0 = first option, 1 = second option, etc.
}

// Init bootstraps the state.
func (m *Model) bootstrapInit() {
	m.bootstrap = &bootstrapState{
		configured: m.state.IsConfigured(),
		cursor:     0,
	}
}

// bootstrapView renders the bootstrap screen.
func bootstrapView(m *Model) string {
	lines := []string{
		Styles.Title.Render("zlaw setup"),
		"",
	}

	if m.bootstrap.configured {
		lines = append(lines, Styles.Heading.Render("Zlaw Home already exists at:"))
		lines = append(lines, Styles.Item.Render(m.state.Home))
		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, bootstrapOption(m, "Re-create", 'R', 0))
		lines = append(lines, bootstrapOption(m, "Keep", 'K', 1))
		lines = append(lines, bootstrapOption(m, "Cancel", 'N', 2))
	} else {
		lines = append(lines, Styles.Heading.Render("Create Zlaw Home?"))
		lines = append(lines, Styles.Item.Render("Path: "+m.state.Home))
		lines = append(lines, "")
		lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
		lines = append(lines, bootstrapOption(m, "Create", 'Y', 0))
		lines = append(lines, bootstrapOption(m, "Cancel", 'N', 1))
	}

	lines = append(lines, "")
	lines = append(lines, Styles.Dim.Render(strings.Repeat("─", 32)))
	lines = append(lines, Styles.Footer.Render("[Q] Quit  [←] Back"))

	return strings.Join(lines, "\n")
}

// bootstrapOption renders a single option.
func bootstrapOption(m *Model, label string, key rune, idx int) string {
	if m.bootstrap.cursor == idx {
		return Styles.Selected.Render("> "+label) + "  [" + string(key) + "]"
	}
	return Styles.Item.Render(label) + "  [" + string(key) + "]"
}

// updateBootstrap handles keyboard events for the bootstrap screen.
func updateBootstrap(m *Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	// Initialize bootstrap state on first update.
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
			// Back to menu.
			m.screen = ScreenMainMenu
			m.bootstrap = nil
			return m, nil

		case "enter", "right", "l":
			m2, cmd := bootstrapConfirm(m)
			return m2, cmd

		case "r", "R":
			if m.bootstrap.configured {
				m.bootstrap.cursor = 0
				m2, cmd := bootstrapConfirm(m)
				return m2, cmd
			}
			return m, nil

		case "n", "N":
			// Cancel - go back to menu.
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
		// Re-create or Keep.
		switch m.bootstrap.cursor {
		case 0: // Re-create
			if err := createZlawHome(m.state.Home, true); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		case 1: // Keep
			// Do nothing, just go back.
		default:
			return m, nil
		}
	} else {
		// Create or Cancel.
		switch m.bootstrap.cursor {
		case 0: // Create
			if err := createZlawHome(m.state.Home, false); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
		default:
			return m, nil
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

// createZlawHome creates the zlaw home structure.
func createZlawHome(home string, force bool) error {
	// Check if already exists.
	zlawPath := filepath.Join(home, "zlaw.toml")
	if _, err := os.Stat(zlawPath); err == nil && !force {
		return nil // Already exists, no need to create.
	}

	// Create directory if needed.
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create home directory: %w", err)
	}

	// Create zlaw.toml.
	zlawTOMLContent := fmt.Sprintf(zlawTOMLTemplate, filepath.Join(home, "agents", "manager"))
	if err := os.WriteFile(zlawPath, []byte(zlawTOMLContent), 0o600); err != nil {
		return fmt.Errorf("write zlaw.toml: %w", err)
	}

	// Create secrets.toml.
	secretsPath := secrets.DefaultSecretsPath()
	if err := secrets.Save(secretsPath, secrets.Store{}); err != nil {
		return fmt.Errorf("create secrets.toml: %w", err)
	}

	// Create agents/ directory.
	agentsDir := filepath.Join(home, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("create agents directory: %w", err)
	}

	return nil
}

// zlawTOMLTemplate has the absolute agent dir substituted for %s.
const zlawTOMLTemplate = `[hub]
name = "main"
description = "zlaw hub"

[[agents]]
id = "manager"
dir = %q
executor = "subprocess"
target = "local"
restart_policy = "on-failure"

	[nats]
`
