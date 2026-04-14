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
	"time"

	"github.com/zsomething/zlaw/internal/logging"
	"github.com/zsomething/zlaw/internal/messaging"
)

// Re-export types from logging package for backward compatibility
type Color = logging.Color

const (
	ColorDefault = logging.ColorDefault
	ColorRed     = logging.ColorRed
	ColorGreen   = logging.ColorGreen
	ColorYellow  = logging.ColorYellow
	ColorBlue    = logging.ColorBlue
	ColorMagenta = logging.ColorMagenta
	ColorCyan    = logging.ColorCyan
	ColorGray    = logging.ColorGray
	ColorWhite   = logging.ColorWhite

	LabelColor = logging.ColorGray
)

var (
	AgentColorPalette = logging.AgentColorPalette
	AgentColor        = logging.AgentColor
	Colorize          = logging.Colorize
)

// DefaultNoColor returns true if output should not use colors.
func DefaultNoColor() bool {
	return logging.DetectNoColor()
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

// ColoredHandler wraps a slog.Handler and adds color and label prefix.
// Deprecated: Use logging.PrettyHandler instead.
type ColoredHandler struct {
	handler slog.Handler
	label   string
	color   Color
	noColor bool
}

// NewColoredHandler creates a new ColoredHandler wrapping the given handler.
func NewColoredHandler(h slog.Handler, noColor *bool) *ColoredHandler {
	nc := logging.DetectNoColor()
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
	var prefix string
	if h.label != "" {
		if h.noColor {
			prefix = h.label + " "
		} else {
			prefix = Colorize(h.color, h.label) + " "
		}
	}
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

// ApplyHandlerOptions applies options to a handler.
func ApplyHandlerOptions(h *ColoredHandler, opts ...ColoredHandlerOption) *ColoredHandler {
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// LinePrefixWriter wraps an io.Writer and prepends a prefix to each line.
type LinePrefixWriter struct {
	w      io.Writer
	prefix string
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

// ColoredLinePrefixWriter adds color to the prefix.
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
	timeColor Color // color for timestamp (gray)
}

// newAgentLogWriter creates a writer that reformats agent JSON logs.
func newAgentLogWriter(label string, color Color, noColor bool) *agentLogWriter {
	return &agentLogWriter{
		label:     label,
		color:     color,
		noColor:   noColor,
		timeColor: ColorGray,
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

	w.buf.Write(p)

	for {
		data := w.buf.String()
		lines := strings.Split(data, "\n")

		if len(lines) <= 1 && !strings.HasSuffix(data, "\n") {
			break
		}

		for i := 0; i < len(lines)-1; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				w.writeLine(line)
			}
		}

		if strings.HasSuffix(data, "\n") {
			w.buf.Reset()
		} else {
			w.buf.Reset()
			w.buf.WriteString(lines[len(lines)-1])
		}
		break //nolint:staticcheck // Only one iteration; process complete lines then return.
	}

	return len(p), nil
}

func (w *agentLogWriter) writeLine(line string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		fmt.Fprintln(os.Stdout, w.prefix("")+line)
		return
	}

	var level, msg, timeStr string
	var attrs []string

	if v, ok := raw["level"]; ok {
		if err := json.Unmarshal(v, &level); err != nil {
			return
		}
	}
	if v, ok := raw["msg"]; ok {
		if err := json.Unmarshal(v, &msg); err != nil {
			return
		}
	}
	if v, ok := raw["time"]; ok {
		if err := json.Unmarshal(v, &timeStr); err != nil {
			return
		}
	}

	agent := w.agentName
	if v, ok := raw["agent"]; ok {
		if err := json.Unmarshal(v, &agent); err != nil {
			return
		}
	}

	for k, v := range raw {
		if k == "time" || k == "level" || k == "msg" || k == "agent" {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			continue
		}
		attrs = append(attrs, k+"="+formatAttr(val))
	}

	// Publish to NATS if messenger is set
	if w.messenger != nil && agent != "" {
		logData := make(map[string]any)
		logData["agent"] = agent
		logData["level"] = level
		logData["msg"] = msg
		if timeStr != "" {
			logData["time"] = timeStr
		}
		for k, v := range raw {
			if k == "time" || k == "level" || k == "msg" || k == "agent" {
				continue
			}
			var val any
			if err := json.Unmarshal(v, &val); err != nil {
				continue
			}
			logData[k] = val
		}
		payload, _ := json.Marshal(logData)
		_ = w.messenger.Publish(context.Background(), "zlaw.logs", payload)
		_ = w.messenger.Publish(context.Background(), fmt.Sprintf("agent.%s.logs", agent), payload)
	}

	var sb strings.Builder

	// Timestamp (from agent JSON or current time)
	if timeStr != "" {
		if t, err := time.Parse(time.RFC3339Nano, timeStr); err == nil {
			timeStr = t.Format("15:04:05")
		}
	} else {
		timeStr = time.Now().Format("15:04:05")
	}

	if w.noColor {
		sb.WriteString(timeStr)
	} else {
		sb.WriteString(Colorize(w.timeColor, timeStr))
	}
	sb.WriteString(" ")

	// Label and level
	sb.WriteString(w.prefix(level))
	sb.WriteString(" ")

	// Message and attrs
	sb.WriteString(msg)
	for _, a := range attrs {
		sb.WriteString(" ")
		sb.WriteString(a)
	}
	fmt.Fprintln(os.Stdout, sb.String())
}

func (w *agentLogWriter) prefix(level string) string {
	var sb strings.Builder

	// Label
	if w.noColor {
		sb.WriteString(w.label)
	} else {
		sb.WriteString(Colorize(w.color, w.label))
	}
	sb.WriteString(" ")

	// Level
	if w.noColor {
		sb.WriteString(strings.ToUpper(level))
	} else {
		levelColor := levelColorFor(level)
		sb.WriteString(Colorize(levelColor, strings.ToUpper(level)))
	}

	return sb.String()
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
//
//nolint:unused
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
