package setup

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles defines the visual styling for the setup wizard.
var Styles = struct {
	// Window is the main container with border
	Window lipgloss.Style

	// Title bar styling
	TitleBar   lipgloss.Style
	TitleLeft  lipgloss.Style
	TitleRight lipgloss.Style

	// Content area
	Heading lipgloss.Style
	Item    lipgloss.Style
	ItemDim lipgloss.Style

	// Selected item
	Selected lipgloss.Style

	// Status indicators
	StatusOK  lipgloss.Style
	StatusErr lipgloss.Style
	StatusDim lipgloss.Style

	// Help text
	Help lipgloss.Style

	// Divider
	Divider lipgloss.Style
}{
	Window: lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B82F6")).
		Padding(0, 1).
		Width(52).
		Align(lipgloss.Center),

	TitleBar: lipgloss.NewStyle().
		Background(lipgloss.Color("#1E3A5F")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true).
		Padding(0, 2).
		Width(50),

	TitleLeft: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true),

	TitleRight: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")),

	Heading: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#60A5FA")).
		Bold(true).
		MarginTop(1),

	Item: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E5E5")).
		Padding(0, 1),

	ItemDim: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")),

	Selected: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#3B82F6")).
		Padding(0, 1),

	StatusOK: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#22C55E")),

	StatusErr: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")),

	StatusDim: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")),

	Help: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginTop(1),

	Divider: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#333333")).
		MarginTop(1),
}

// windowView wraps content in a window-like frame with header
func windowView(title string, content string, help string) string {
	var lines []string

	// Title bar
	lines = append(lines, Styles.TitleBar.Render("  "+title))
	lines = append(lines, "")

	// Content
	lines = append(lines, content)

	// Help text
	if help != "" {
		lines = append(lines, "")
		lines = append(lines, Styles.Help.Render(help))
	}

	return strings.Join(lines, "\n")
}

// statusView renders a status indicator
func statusView(state ItemState) string {
	switch state {
	case StateConfigured:
		return Styles.StatusOK.Render("✓")
	case StateMissing:
		return Styles.StatusErr.Render("✗")
	case StateInvalid:
		return Styles.StatusErr.Render("✗")
	default:
		return Styles.StatusDim.Render("·")
	}
}
