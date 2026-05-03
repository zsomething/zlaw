package setup

import (
	"github.com/charmbracelet/bubbletea"
)

// SetupCmd is the entry point for the interactive setup wizard.
type SetupCmd struct{}

// Run launches the Bubble Tea TUI.
func (c *SetupCmd) Run() error {
	m := &Model{
		state:  LoadAppState(),
		screen: ScreenMainMenu,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
