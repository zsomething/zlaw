package hub

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Color represents ANSI terminal colors.
type Color int

const (
	ColorDefault Color = 0
	ColorRed     Color = 31
	ColorGreen   Color = 32
	ColorYellow  Color = 33
	ColorBlue    Color = 34
	ColorMagenta Color = 35
	ColorCyan    Color = 36
	ColorGray    Color = 90
	ColorWhite   Color = 97
)

// AgentColorPalette is a stable color palette for agent log prefixes.
var AgentColorPalette = []Color{
	ColorCyan,
	ColorGreen,
	ColorMagenta,
	ColorYellow,
	ColorBlue,
	ColorRed,
}

// AgentColor returns a stable color for the given agent name.
// Uses a simple hash to select from the palette deterministically.
func AgentColor(name string) Color {
	hash := 0
	for _, c := range name {
		hash = hash*31 + int(c)
	}
	return AgentColorPalette[hash%len(AgentColorPalette)]
}

// Colorize applies ANSI color codes to text if color is enabled.
func Colorize(color Color, text string) string {
	if color == ColorDefault {
		return text
	}
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", color, text)
}

// LabelColor is the color used for hub log lines.
const LabelColor = ColorGray

// DefaultNoColor returns true if output should not use colors.
// Respects ZLAW_NO_COLOR env var and checks if stdout is a TTY.
func DefaultNoColor() bool {
	if os.Getenv("ZLAW_NO_COLOR") != "" {
		return true
	}
	return !isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	// Try to detect if file descriptor is a terminal.
	// This is a simple check; works on Linux/macOS.
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}

// ColoredHandler wraps a slog.Handler and adds color and label prefix.
type ColoredHandler struct {
	handler slog.Handler
	label   string
	color   Color
	noColor bool
	attrs   []slog.Attr
	groups  bool
}

// ColoredHandlerOption configures a ColoredHandler.
type ColoredHandlerOption func(*ColoredHandler)

// WithLabel sets the prefix label (e.g., "[hub]" or "[agentname]").
func WithLabel(label string) ColoredHandlerOption {
	return func(h *ColoredHandler) { h.label = label }
}

// WithColor sets the ANSI color code for the label.
func WithColor(color Color) ColoredHandlerOption {
	return func(h *ColoredHandler) { h.color = color }
}

// WithNoColor disables ANSI color output.
func WithNoColor(noColor bool) ColoredHandlerOption {
	return func(h *ColoredHandler) { h.noColor = noColor }
}

// NewColoredHandler creates a new ColoredHandler wrapping the given handler.
// If noColor is nil, defaults to DefaultNoColor().
func NewColoredHandler(h slog.Handler, noColor *bool) *ColoredHandler {
	nc := DefaultNoColor()
	if noColor != nil {
		nc = *noColor
	}
	return &ColoredHandler{
		handler: h,
		color:   LabelColor,
		noColor: nc,
	}
}

// Handle implements slog.Handler.
func (h *ColoredHandler) Handle(ctx context.Context, r slog.Record) error {
	// Build the prefix
	var prefix string
	if h.label != "" {
		if h.noColor {
			prefix = h.label + " "
		} else {
			prefix = Colorize(h.color, h.label) + " "
		}
	}

	// Get the formatted message from the underlying handler
	// We need to intercept the write to add our prefix.
	// Since slog.TextHandler writes directly, we'll capture it.
	return h.handleWithPrefix(ctx, r, prefix)
}

func (h *ColoredHandler) handleWithPrefix(ctx context.Context, r slog.Record, prefix string) error {
	// Modify the message to include the prefix before passing to handler.
	if prefix != "" {
		r.Message = prefix + r.Message
	}
	return h.handler.Handle(ctx, r)
}

// Enabled implements slog.Handler.
func (h *ColoredHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// WithAttrs implements slog.Handler.
func (h *ColoredHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ColoredHandler{
		handler: h.handler.WithAttrs(attrs),
		label:   h.label,
		color:   h.color,
		noColor: h.noColor,
	}
}

// WithGroup implements slog.Handler.
func (h *ColoredHandler) WithGroup(name string) slog.Handler {
	return &ColoredHandler{
		handler: h.handler.WithGroup(name),
		label:   h.label,
		color:   h.color,
		noColor: h.noColor,
	}
}

// LinePrefixWriter wraps an io.Writer and prepends a prefix to each line.
type LinePrefixWriter struct {
	w       io.Writer
	prefix  string
	noColor bool
}

// NewLinePrefixWriter creates a writer that prefixes each line.
func NewLinePrefixWriter(w io.Writer, prefix string) *LinePrefixWriter {
	return &LinePrefixWriter{w: w, prefix: prefix}
}

// Write implements io.Writer.
func (p *LinePrefixWriter) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	lines := strings.Split(string(b), "\n")
	for i, line := range lines {
		if line != "" || i < len(lines)-1 {
			_, err := p.w.Write([]byte(p.prefix))
			if err != nil {
				return 0, err
			}
			_, err = p.w.Write([]byte(line))
			if err != nil {
				return 0, err
			}
		}
		if i < len(lines)-1 {
			_, err = p.w.Write([]byte("\n"))
			if err != nil {
				return 0, err
			}
		}
	}
	return len(b), nil
}

// ColoredLinePrefixWriter adds color to the prefix when not in no-color mode.
type ColoredLinePrefixWriter struct {
	*LinePrefixWriter
	color Color
}

// NewColoredLinePrefixWriter creates a line prefix writer with color support.
func NewColoredLinePrefixWriter(w io.Writer, prefix string, color Color, noColor bool) *ColoredLinePrefixWriter {
	p := prefix
	if !noColor && color != ColorDefault {
		p = Colorize(color, prefix)
	}
	return &ColoredLinePrefixWriter{
		LinePrefixWriter: NewLinePrefixWriter(w, p),
		color:            color,
	}
}

// ApplyHandlerOptions applies options to a handler and returns it.
func ApplyHandlerOptions(h *ColoredHandler, opts ...ColoredHandlerOption) *ColoredHandler {
	for _, opt := range opts {
		opt(h)
	}
	return h
}
