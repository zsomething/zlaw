package logging

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// JSONLogEntry represents a structured log entry from an agent.
type JSONLogEntry struct {
	Time  string
	Level string
	Msg   string
	Agent string
	Attrs map[string]any
}

// ParseJSONLogEntry parses a single line of JSON output.
func ParseJSONLogEntry(line string) (*JSONLogEntry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}

	entry := &JSONLogEntry{Attrs: make(map[string]any)}

	if v, ok := raw["time"]; ok {
		json.Unmarshal(v, &entry.Time)
	}
	if v, ok := raw["level"]; ok {
		json.Unmarshal(v, &entry.Level)
	}
	if v, ok := raw["msg"]; ok {
		json.Unmarshal(v, &entry.Msg)
	}
	if v, ok := raw["agent"]; ok {
		json.Unmarshal(v, &entry.Agent)
	}

	// Collect remaining fields as attributes
	for k, v := range raw {
		if k == "time" || k == "level" || k == "msg" || k == "agent" {
			continue
		}
		var val any
		json.Unmarshal(v, &val)
		entry.Attrs[k] = val
	}

	return entry, nil
}

// LevelFromString converts a level string to slog.Level.
func LevelFromString(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// FormatEntry formats a JSONLogEntry as a pretty log line.
func FormatEntry(entry *JSONLogEntry, label string, color Color, noColor bool) string {
	if entry == nil {
		return ""
	}

	var sb strings.Builder

	// Label
	if label != "" {
		if noColor {
			sb.WriteString(label)
		} else {
			sb.WriteString(Colorize(color, label))
		}
		sb.WriteString(" ")
	}

	// Level with color
	level := LevelFromString(entry.Level)
	levelColor := LevelColors[level]
	levelStr := strings.ToUpper(entry.Level)
	if levelStr == "WARNING" {
		levelStr = "WARN" // slog uses WARN
	}
	if noColor {
		sb.WriteString(strings.ToUpper(levelStr))
	} else {
		sb.WriteString(Colorize(levelColor, strings.ToUpper(levelStr)))
	}
	sb.WriteString("  ")

	// Message
	sb.WriteString(entry.Msg)

	// Attributes
	for k, v := range entry.Attrs {
		sb.WriteString("  ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(formatValue(v))
	}

	return sb.String()
}

func formatValue(v any) string {
	switch val := v.(type) {
	case string:
		// Quote strings that contain spaces
		if strings.Contains(val, " ") || strings.Contains(val, "=") {
			return fmt.Sprintf("%q", val)
		}
		return val
	case nil:
		return "<nil>"
	default:
		return fmt.Sprintf("%v", val)
	}
}

// JSONLogReader reads and reformats JSON log lines from an io.Reader.
type JSONLogReader struct {
	scanner *bufio.Scanner
	label   string
	color   Color
	noColor bool
	onEntry func(*JSONLogEntry) // called for each parsed entry
	onError func(error)         // called on parse errors
}

// NewJSONLogReader creates a reader that reformats JSON logs.
func NewJSONLogReader(r io.Reader, label string, color Color, noColor bool) *JSONLogReader {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for long log lines
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	return &JSONLogReader{
		scanner: scanner,
		label:   label,
		color:   color,
		noColor: noColor,
	}
}

// SetOnEntry sets the callback for parsed entries.
func (r *JSONLogReader) SetOnEntry(fn func(*JSONLogEntry)) {
	r.onEntry = fn
}

// SetOnError sets the callback for parse errors.
func (r *JSONLogReader) SetOnError(fn func(error)) {
	r.onError = fn
}

// Run starts reading and processing lines. Blocks until reader is exhausted.
func (r *JSONLogReader) Run() {
	for r.scanner.Scan() {
		line := r.scanner.Text()
		entry, err := ParseJSONLogEntry(line)
		if err != nil {
			if r.onError != nil {
				r.onError(err)
			}
			continue
		}
		if entry == nil {
			continue
		}
		if r.onEntry != nil {
			r.onEntry(entry)
		}
	}
}

// WriterToJSONHandler returns an io.Writer that captures log lines.
type WriterToJSONHandler struct {
	onLine func(string)
}

// NewWriterToJSONHandler creates a writer adapter.
func NewWriterToJSONHandler(onLine func(string)) *WriterToJSONHandler {
	return &WriterToJSONHandler{onLine: onLine}
}

func (w *WriterToJSONHandler) Write(p []byte) (n int, err error) {
	w.onLine(string(p))
	return len(p), nil
}
