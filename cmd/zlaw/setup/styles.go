package setup

import "github.com/charmbracelet/lipgloss"

// Color palette from design spec.
const (
	ColorHeaderBg = "#1a1a2e"
	ColorAccent   = "#00aaee"
	ColorDim      = "#666666"
	ColorText     = "#cccccc"
	ColorSuccess  = "#00cc66"
	ColorWarning  = "#ffaa00"
	ColorError    = "#ff4444"
	ColorBorder   = "#334455"
)

// Global frame width for consistent layout.
const FrameWidth = 58

// Styles defines all visual styling.
var Styles = struct {
	Header       lipgloss.Style
	Footer       lipgloss.Style
	SectionLabel lipgloss.Style
	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	ItemDim      lipgloss.Style
	StatusOK     lipgloss.Style
	StatusWarn   lipgloss.Style
	StatusErr    lipgloss.Style
	StatusDim    lipgloss.Style
	HelpKey      lipgloss.Style
	Success      lipgloss.Style
}{
	Header: lipgloss.NewStyle().
		Background(lipgloss.Color(ColorHeaderBg)).
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true),

	Footer: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDim)),

	SectionLabel: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)).
		Bold(true),

	Item: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorText)),

	ItemSelected: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color(ColorAccent)).
		Bold(true),

	ItemDim: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDim)),

	StatusOK: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSuccess)),

	StatusWarn: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)),

	StatusErr: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorError)),

	StatusDim: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorDim)),

	HelpKey: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)),

	Success: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSuccess)),
}

// statusText returns the text for an ItemState.
func statusText(s ItemState) string {
	switch s {
	case StateMissing:
		return "missing"
	case StateConfigured:
		return "configured"
	case StateInvalid:
		return "invalid"
	default:
		return ""
	}
}

// statusStyle returns the lipgloss style for an ItemState.
func statusStyle(s ItemState) lipgloss.Style {
	switch s {
	case StateMissing:
		return Styles.StatusWarn
	case StateConfigured:
		return Styles.StatusOK
	case StateInvalid:
		return Styles.StatusErr
	default:
		return Styles.StatusDim
	}
}

// bootstrapStatusText returns the status text for a BootstrapStatus.
func bootstrapStatusText(s BootstrapStatus) (text string, style lipgloss.Style) {
	switch s {
	case BootstrapNotReady:
		return "⚠️ not initialized", Styles.StatusWarn
	case BootstrapReady:
		return "✅ configured", Styles.StatusOK
	case BootstrapIncomplete:
		return "⚠️ incomplete setup", Styles.StatusWarn
	default:
		return "", Styles.StatusDim
	}
}
