package setup

import "github.com/charmbracelet/lipgloss"

// Styles defines the visual styling for the setup wizard.
var Styles = struct {
	Title     lipgloss.Style
	Heading   lipgloss.Style
	Item      lipgloss.Style
	ItemHelp  lipgloss.Style
	Selected  lipgloss.Style
	StatusOK  lipgloss.Style
	StatusErr lipgloss.Style
	StatusDim lipgloss.Style
	Dim       lipgloss.Style
	Border    lipgloss.Style
	Footer    lipgloss.Style
}{
	Title: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Bold(true),

	Heading: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		Bold(true),

	Item: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#CCCCCC")),

	ItemHelp: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")),

	Selected: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#3B82F6")).
		Bold(false).
		Padding(0, 1),

	StatusOK: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#22C55E")),

	StatusErr: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")),

	StatusDim: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")),

	Dim: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")),

	Border: lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B82F6")).
		Padding(1, 2),

	Footer: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Padding(0, 1),
}
