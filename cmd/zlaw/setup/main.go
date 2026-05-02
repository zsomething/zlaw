package setup

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
)

// SetupCmd is the entry point for the interactive setup wizard.
type SetupCmd struct{}

// Run launches the Bubble Tea TUI and blocks until the user quits.
func (c *SetupCmd) Run() error {
	state, err := LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	m := Model{
		state:  state,
		screen: ScreenMainMenu, // Init() will adjust if not configured
	}

	p := tea.NewProgram(&m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return fmt.Errorf("run setup: %w", err)
	}

	// Propagate quit signal if needed.
	if m, ok := result.(*Model); ok && m.quit {
		return os.ErrExist
	}

	return nil
}
