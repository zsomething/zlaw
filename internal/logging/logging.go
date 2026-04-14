// Package logging provides structured logging utilities with pretty output
// and JSON mode for hub/agent log aggregation.
package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"
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

// LevelColors maps log levels to their display colors.
var LevelColors = map[slog.Level]Color{
	slog.LevelDebug: ColorGray,
	slog.LevelInfo:  ColorGreen,
	slog.LevelWarn:  ColorYellow,
	slog.LevelError: ColorRed,
}

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
func AgentColor(name string) Color {
	hash := 0
	for _, c := range name {
		hash = hash*31 + int(c)
	}
	return AgentColorPalette[hash%len(AgentColorPalette)]
}

// Colorize applies ANSI color codes to text.
func Colorize(color Color, text string) string {
	if color == ColorDefault {
		return text
	}
	return fmt.Sprintf("\x1b[%dm%s\x1b[0m", color, text)
}

// TimeFormat controls time display in pretty logs.
type TimeFormat int

const (
	TimeFormatNone  TimeFormat = iota // no time shown
	TimeFormatShort                   // HH:MM:SS
	TimeFormatFull                    // 2006-01-02T15:04:05Z07:00
)

// LogFormat specifies the output format.
type LogFormat string

const (
	LogFormatPretty LogFormat = "pretty"
	LogFormatJSON   LogFormat = "json"
)

// DetectFormat returns the appropriate log format based on environment.
func DetectFormat() LogFormat {
	if os.Getenv("ZLAW_LOG_FORMAT") == "json" {
		return LogFormatJSON
	}
	return LogFormatPretty
}

// DetectNoColor returns true if colors should be disabled.
func DetectNoColor() bool {
	if os.Getenv("ZLAW_NO_COLOR") != "" {
		return true
	}
	return !isTTY(os.Stdout)
}

// DetectTimeFormat returns the time format based on ZLAW_LOG_TIME env var.
func DetectTimeFormat() TimeFormat {
	switch strings.ToLower(os.Getenv("ZLAW_LOG_TIME")) {
	case "none":
		return TimeFormatNone
	case "short":
		return TimeFormatShort
	case "full":
		return TimeFormatFull
	default:
		return TimeFormatShort
	}
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice
}

// Options configures a Handler.
type Options struct {
	Label   string     // e.g., "[hub]" or "[agent:name]"
	Color   Color      // color for label
	NoColor bool       // disable colors
	Time    TimeFormat // time display format
}

// PrettyHandler implements slog.Handler with pretty colored output.
// It outputs logs in the format: [label] LEVEL message key=value...
type PrettyHandler struct {
	w    io.Writer
	opts Options
}

// NewPrettyHandler creates a PrettyHandler that writes to w (defaults to stderr).
func NewPrettyHandler(w io.Writer, opts Options) *PrettyHandler {
	if w == nil {
		w = os.Stderr
	}
	return &PrettyHandler{w: w, opts: opts}
}

// Handle formats the log record as a pretty line and writes it.
func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder

	// Time
	if h.opts.Time != TimeFormatNone {
		var timeStr string
		switch h.opts.Time {
		case TimeFormatShort:
			timeStr = r.Time.Format("15:04:05")
		case TimeFormatFull:
			timeStr = r.Time.Format(time.RFC3339)
		}
		if h.opts.NoColor {
			sb.WriteString(timeStr)
		} else {
			sb.WriteString(Colorize(ColorGray, timeStr))
		}
		sb.WriteString("  ")
	}

	// Label
	if h.opts.Label != "" {
		if h.opts.NoColor {
			sb.WriteString(h.opts.Label)
		} else {
			sb.WriteString(Colorize(h.opts.Color, h.opts.Label))
		}
		sb.WriteString("  ")
	}

	// Level
	levelStr := strings.ToUpper(r.Level.String())
	if h.opts.NoColor {
		sb.WriteString(fmt.Sprintf("%-5s", levelStr))
	} else {
		color := LevelColors[r.Level]
		sb.WriteString(Colorize(color, fmt.Sprintf("%-5s", levelStr)))
	}
	sb.WriteString("  ")

	// Message
	sb.WriteString(r.Message)

	// Attributes
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Key < attrs[j].Key
	})

	for _, a := range attrs {
		sb.WriteString("  ")
		sb.WriteString(a.Key)
		sb.WriteString("=")
		sb.WriteString(formatAttrValue(a.Value))
	}

	sb.WriteString("\n")
	_, err := h.w.Write([]byte(sb.String()))
	return err
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For now, just return self - attrs are printed inline
	_ = attrs
	return h
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	// Groups are not supported in pretty mode
	_ = name
	return h
}

func formatAttrValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		// Quote strings with spaces
		if strings.ContainsAny(s, " \t\n\"") {
			return fmt.Sprintf("%q", s)
		}
		return s
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%t", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny, slog.KindLogValuer:
		return fmt.Sprintf("%v", v.Any())
	default:
		return v.String()
	}
}

// JSONHandler implements slog.Handler with JSON output.
type JSONHandler struct {
	w      io.Writer
	attrs  []slog.Attr
	groups []string
}

// NewJSONHandler creates a JSON handler that writes to w (defaults to stdout).
func NewJSONHandler(w io.Writer) *JSONHandler {
	if w == nil {
		w = os.Stdout
	}
	return &JSONHandler{w: w}
}

func (h *JSONHandler) Handle(_ context.Context, r slog.Record) error {
	m := make(map[string]any, 4+r.NumAttrs())
	m["time"] = r.Time.Format(time.RFC3339Nano)
	m["level"] = r.Level.String()
	m["msg"] = r.Message
	if agent := os.Getenv("ZLAW_AGENT"); agent != "" {
		m["agent"] = agent
	}

	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})

	enc := json.NewEncoder(h.w)
	enc.SetEscapeHTML(false)
	enc.Encode(m)
	return nil
}

func (h *JSONHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *JSONHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &JSONHandler{w: h.w, attrs: append(h.attrs, attrs...), groups: h.groups}
}

func (h *JSONHandler) WithGroup(name string) slog.Handler {
	return &JSONHandler{w: h.w, attrs: h.attrs, groups: append(h.groups, name)}
}

// Logger returns a *slog.Logger configured with the appropriate handler.
func Logger(label string, color Color) *slog.Logger {
	opts := Options{
		Label:   label,
		Color:   color,
		NoColor: DetectNoColor(),
		Time:    DetectTimeFormat(),
	}

	var h slog.Handler
	if DetectFormat() == LogFormatJSON {
		h = NewJSONHandler(os.Stdout)
	} else {
		h = NewPrettyHandler(os.Stdout, opts)
	}

	return slog.New(h)
}

// LoggerWithOptions returns a *slog.Logger with explicit options.
func LoggerWithOptions(opts Options) *slog.Logger {
	var h slog.Handler
	if DetectFormat() == LogFormatJSON {
		h = NewJSONHandler(os.Stdout)
	} else {
		h = NewPrettyHandler(os.Stdout, opts)
	}

	return slog.New(h)
}
