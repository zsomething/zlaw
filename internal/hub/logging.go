package hub

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/zsomething/zlaw/internal/messaging"
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

// agentLogWriter reads JSON log lines from the agent process and writes
// them in pretty format to stdout and optionally publishes to NATS.
type agentLogWriter struct {
	label     string
	color     Color
	noColor   bool
	buf       strings.Builder
	mu        sync.Mutex
	messenger messaging.Messenger
	agentName string
}

// newAgentLogWriter creates a writer that reformats agent JSON logs.
func newAgentLogWriter(label string, color Color, noColor bool) *agentLogWriter {
	return &agentLogWriter{
		label:   label,
		color:   color,
		noColor: noColor,
	}
}

// withMessenger sets the messenger for NATS publishing.
func (w *agentLogWriter) withMessenger(m messaging.Messenger, agentName string) *agentLogWriter {
	w.messenger = m
	w.agentName = agentName
	return w
}

func (w *agentLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Accumulate bytes until we have a complete line
	w.buf.Write(p)

	for {
		data := w.buf.String()
		lines := strings.Split(data, "\n")

		// If we don't have a complete line, break
		if len(lines) <= 1 && !strings.HasSuffix(data, "\n") {
			break
		}

		// Process all complete lines
		for i := 0; i < len(lines)-1; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				w.writeLine(line)
			}
		}

		// Keep incomplete line in buffer
		if strings.HasSuffix(data, "\n") {
			w.buf.Reset()
		} else {
			w.buf.Reset()
			w.buf.WriteString(lines[len(lines)-1])
		}
		break
	}

	return len(p), nil
}

func (w *agentLogWriter) writeLine(line string) {
	// Try to parse as JSON log entry
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		// Not JSON, just print as-is with prefix
		fmt.Fprintln(os.Stdout, w.prefix("")+line)
		return
	}

	// Extract fields
	var level, msg string
	var attrs []string

	if v, ok := raw["level"]; ok {
		json.Unmarshal(v, &level)
	}
	if v, ok := raw["msg"]; ok {
		json.Unmarshal(v, &msg)
	}

	// Build agent field from agentName if not in raw
	agent := w.agentName
	if v, ok := raw["agent"]; ok {
		json.Unmarshal(v, &agent)
	}

	for k, v := range raw {
		if k == "time" || k == "level" || k == "msg" || k == "agent" {
			continue
		}
		var val any
		json.Unmarshal(v, &val)
		attrs = append(attrs, k+"="+formatAttr(val))
	}

	// Publish to NATS if messenger is set
	if w.messenger != nil && agent != "" {
		// Reconstruct JSON with agent field for NATS publishing
		logData := make(map[string]any)
		logData["agent"] = agent
		logData["level"] = level
		logData["msg"] = msg
		if t, ok := raw["time"]; ok {
			var timeStr string
			json.Unmarshal(t, &timeStr)
			logData["time"] = timeStr
		}
		for k, v := range raw {
			if k == "time" || k == "level" || k == "msg" || k == "agent" {
				continue
			}
			var val any
			json.Unmarshal(v, &val)
			logData[k] = val
		}
		payload, _ := json.Marshal(logData)
		// Publish to both global and per-agent subjects
		w.messenger.Publish(context.Background(), "zlaw.logs", payload)
		w.messenger.Publish(context.Background(), fmt.Sprintf("agent.%s.logs", agent), payload)
	}

	// Format: [agent:name] LEVEL msg  key=value...
	var sb strings.Builder
	sb.WriteString(w.prefix(level))
	sb.WriteString("  ")
	sb.WriteString(msg)
	for _, a := range attrs {
		sb.WriteString("  ")
		sb.WriteString(a)
	}
	fmt.Fprintln(os.Stdout, sb.String())
}

func (w *agentLogWriter) prefix(level string) string {
	if w.noColor {
		return w.label + " " + strings.ToUpper(fmt.Sprintf("%-5s", level))
	}

	coloredLabel := Colorize(w.color, w.label)

	levelColor := levelColorFor(level)
	coloredLevel := Colorize(levelColor, strings.ToUpper(fmt.Sprintf("%-5s", level)))

	return coloredLabel + " " + coloredLevel
}

func levelColorFor(level string) Color {
	switch strings.ToLower(level) {
	case "debug":
		return ColorGray
	case "info":
		return ColorGreen
	case "warn", "warning":
		return ColorYellow
	case "error":
		return ColorRed
	default:
		return ColorDefault
	}
}

func formatAttr(v any) string {
	switch val := v.(type) {
	case string:
		if strings.Contains(val, " ") || strings.Contains(val, "=") || strings.Contains(val, "\"") {
			return fmt.Sprintf("%q", val)
		}
		return val
	case nil:
		return "<nil>"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// pipeAgentLogs spawns a goroutine that reads from r and writes pretty logs.
// Returns a writer that the caller should set as the process stdout/stderr.
func pipeAgentLogs(r io.Reader, label string, color Color, noColor bool) io.Writer {
	pr, pw := io.Pipe()
	w := newAgentLogWriter(label, color, noColor)

	go func() {
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				w.writeLine(line)
			}
		}
	}()

	return pw
}
