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

// Level colors for pretty output.
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
		return TimeFormatShort // default to short
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
	Label    string // e.g., "[hub]" or "[agent:name]"
	Color    Color  // color for label
	NoColor  bool   // disable colors
	Time     TimeFormat
	LevelKey string // attribute key for level (default "level")
	TimeKey  string // attribute key for time (default "time")
	MsgKey   string // attribute key for message (default "msg")
}

// PrettyHandler implements slog.Handler with pretty colored output.
type PrettyHandler struct {
	handler slog.Handler
	opts    Options
	buf     strings.Builder
}

// NewPrettyHandler creates a PrettyHandler wrapping the default text handler.
func NewPrettyHandler(w io.Writer, opts Options) *PrettyHandler {
	if w == nil {
		w = os.Stderr
	}
	replacer := func(_ []string, a slog.Attr) slog.Attr {
		if opts.Time == TimeFormatNone && a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		if opts.Time == TimeFormatShort && a.Key == slog.TimeKey {
			return slog.Attr{Key: a.Key, Value: slog.StringValue(a.Value.Time().Format("15:04:05"))}
		}
		return a
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		ReplaceAttr: replacer,
	})
	return &PrettyHandler{
		handler: h,
		opts:    opts,
	}
}

// NewPrettyHandlerFrom wraps an existing slog.Handler with pretty prefix.
func NewPrettyHandlerFrom(h slog.Handler, opts Options) *PrettyHandler {
	return &PrettyHandler{
		handler: h,
		opts:    opts,
	}
}

func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	// Build the label prefix with color
	var labelPrefix string
	if h.opts.Label != "" {
		if h.opts.NoColor {
			labelPrefix = h.opts.Label + " "
		} else {
			labelPrefix = Colorize(h.opts.Color, h.opts.Label) + " "
		}
	}

	// Get level color
	levelColor := LevelColors[r.Level]
	if h.opts.NoColor {
		// Just use the level string
		levelStr := r.Level.String()
		// Pad to 5 chars for alignment
		levelStr = fmt.Sprintf("%-5s", levelStr)
		if labelPrefix != "" {
			r.Message = labelPrefix + levelStr + " " + r.Message
		} else {
			r.Message = levelStr + " " + r.Message
		}
	} else {
		levelStr := Colorize(levelColor, fmt.Sprintf("%-5s", r.Level.String()))
		if labelPrefix != "" {
			r.Message = labelPrefix + levelStr + " " + r.Message
		} else {
			r.Message = levelStr + " " + r.Message
		}
	}

	return h.handler.Handle(ctx, r)
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.handler.Enabled(context.Background(), level)
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{
		handler: h.handler.WithAttrs(attrs),
		opts:    h.opts,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{
		handler: h.handler.WithGroup(name),
		opts:    h.opts,
	}
}

// JSONHandler implements slog.Handler with JSON output.
type JSONHandler struct {
	attrs  []slog.Attr
	groups []string
}

// NewJSONHandler creates a JSON handler.
func NewJSONHandler(w io.Writer) *JSONHandler {
	return &JSONHandler{}
}

func (h *JSONHandler) Handle(_ context.Context, r slog.Record) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	m := make(map[string]any, 4+r.NumAttrs())
	m["time"] = r.Time.Format(time.RFC3339Nano)
	m["level"] = r.Level.String()
	m["msg"] = r.Message
	m["agent"] = os.Getenv("ZLAW_AGENT")

	// Add attributes
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})

	return enc.Encode(m)
}

func (h *JSONHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *JSONHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &JSONHandler{attrs: attrs}
}

func (h *JSONHandler) WithGroup(name string) slog.Handler {
	return &JSONHandler{groups: append(h.groups, name)}
}

// Logger returns a *slog.Logger configured with the appropriate handler.
func Logger(label string, color Color) *slog.Logger {
	opts := Options{
		Label:   label,
		Color:   color,
		NoColor: DetectNoColor(),
		Time:    DetectTimeFormat(),
	}

	format := DetectFormat()
	var h slog.Handler

	if format == LogFormatJSON {
		h = NewJSONHandler(os.Stdout)
	} else {
		h = NewPrettyHandler(os.Stdout, opts)
	}

	return slog.New(h)
}

// LoggerWithOptions returns a *slog.Logger with explicit options.
func LoggerWithOptions(opts Options) *slog.Logger {
	format := DetectFormat()
	var h slog.Handler

	if format == LogFormatJSON {
		h = NewJSONHandler(os.Stdout)
	} else {
		h = NewPrettyHandler(os.Stdout, opts)
	}

	return slog.New(h)
}
