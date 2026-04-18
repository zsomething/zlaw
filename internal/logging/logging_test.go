package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/logging"
)

func TestColorize_Default(t *testing.T) {
	got := logging.Colorize(logging.ColorDefault, "hello")
	if got != "hello" {
		t.Errorf("Colorize(ColorDefault, \"hello\") = %q, want %q", got, "hello")
	}
}

func TestColorize_WithColor(t *testing.T) {
	got := logging.Colorize(logging.ColorRed, "error")
	want := "\x1b[31merror\x1b[0m"
	if got != want {
		t.Errorf("Colorize(ColorRed, \"error\") = %q, want %q", got, want)
	}
}

func TestColorize_EmptyString(t *testing.T) {
	// Colorize applies color codes even to empty strings
	got := logging.Colorize(logging.ColorGreen, "")
	want := "\x1b[32m\x1b[0m"
	if got != want {
		t.Errorf("Colorize(ColorGreen, \"\") = %q, want %q", got, want)
	}
}

func TestAgentColor_Stable(t *testing.T) {
	// Same name should always return same color
	name := "test-agent"
	c1 := logging.AgentColor(name)
	c2 := logging.AgentColor(name)
	if c1 != c2 {
		t.Errorf("AgentColor(%q) not stable: got %v then %v", name, c1, c2)
	}
}

func TestAgentColor_DifferentNames(t *testing.T) {
	// Different names may return same or different colors (hash collision possible)
	// Colors are valid ANSI codes in range 30-97
	names := []string{"agent1", "agent2", "agent3", "agent4", "agent5", "agent6", "agent7"}
	for _, name := range names {
		c := logging.AgentColor(name)
		if c < 30 || c > 97 {
			t.Errorf("AgentColor(%q) = %v, outside valid ANSI range 30-97", name, c)
		}
	}
}

func TestAgentColor_KnownInputs(t *testing.T) {
	// Verify specific names produce colors in valid range
	tests := []struct {
		name string
	}{
		{name: "alice"},
		{name: "bob"},
		{name: ""}, // empty string edge case
	}

	for _, tt := range tests {
		c := logging.AgentColor(tt.name)
		if c < 30 || c > 97 {
			t.Errorf("AgentColor(%q) = %v, want valid ANSI color 30-97", tt.name, c)
		}
	}
}

func TestDetectFormat_Pretty(t *testing.T) {
	os.Setenv("ZLAW_LOG_FORMAT", "")
	got := logging.DetectFormat()
	if got != logging.LogFormatPretty {
		t.Errorf("DetectFormat() = %v, want %v", got, logging.LogFormatPretty)
	}
}

func TestDetectFormat_JSON(t *testing.T) {
	os.Setenv("ZLAW_LOG_FORMAT", "json")
	defer os.Unsetenv("ZLAW_LOG_FORMAT")
	got := logging.DetectFormat()
	if got != logging.LogFormatJSON {
		t.Errorf("DetectFormat() with ZLAW_LOG_FORMAT=json = %v, want %v", got, logging.LogFormatJSON)
	}
}

func TestDetectFormat_CaseSensitive(t *testing.T) {
	// DetectFormat is case-sensitive
	os.Setenv("ZLAW_LOG_FORMAT", "JSON")
	defer os.Unsetenv("ZLAW_LOG_FORMAT")
	got := logging.DetectFormat()
	// Should return pretty since "JSON" != "json"
	if got != logging.LogFormatPretty {
		t.Errorf("DetectFormat() with ZLAW_LOG_FORMAT=JSON = %v, want %v", got, logging.LogFormatPretty)
	}
}

func TestDetectTimeFormat_Default(t *testing.T) {
	os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatShort {
		t.Errorf("DetectTimeFormat() default = %v, want %v", got, logging.TimeFormatShort)
	}
}

func TestDetectTimeFormat_None(t *testing.T) {
	os.Setenv("ZLAW_LOG_TIME", "none")
	defer os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatNone {
		t.Errorf("DetectTimeFormat() with none = %v, want %v", got, logging.TimeFormatNone)
	}
}

func TestDetectTimeFormat_Short(t *testing.T) {
	os.Setenv("ZLAW_LOG_TIME", "short")
	defer os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatShort {
		t.Errorf("DetectTimeFormat() with short = %v, want %v", got, logging.TimeFormatShort)
	}
}

func TestDetectTimeFormat_Full(t *testing.T) {
	os.Setenv("ZLAW_LOG_TIME", "full")
	defer os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatFull {
		t.Errorf("DetectTimeFormat() with full = %v, want %v", got, logging.TimeFormatFull)
	}
}

func TestDetectTimeFormat_CaseInsensitive(t *testing.T) {
	os.Setenv("ZLAW_LOG_TIME", "FULL")
	defer os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatFull {
		t.Errorf("DetectTimeFormat() with FULL = %v, want %v", got, logging.TimeFormatFull)
	}
}

func TestDetectTimeFormat_InvalidValue(t *testing.T) {
	os.Setenv("ZLAW_LOG_TIME", "invalid")
	defer os.Unsetenv("ZLAW_LOG_TIME")
	got := logging.DetectTimeFormat()
	if got != logging.TimeFormatShort {
		t.Errorf("DetectTimeFormat() with invalid = %v, want %v", got, logging.TimeFormatShort)
	}
}

func TestPrettyHandler_Handle_Basic(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewPrettyHandler(&buf, logging.Options{
		Label:   "[test]",
		Color:   logging.ColorGreen,
		NoColor: true,
		Time:    logging.TimeFormatNone,
	})

	rec := slog.Record{
		Time:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Message: "hello world",
		Level:   slog.LevelInfo,
	}
	h.Handle(context.Background(), rec)

	got := buf.String()
	if !strings.Contains(got, "[test]") {
		t.Errorf("Handle output missing label: %s", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("Handle output missing message: %s", got)
	}
	if !strings.Contains(got, "INFO") {
		t.Errorf("Handle output missing level: %s", got)
	}
}

func TestPrettyHandler_Handle_WithTime(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewPrettyHandler(&buf, logging.Options{
		Label:   "",
		NoColor: true,
		Time:    logging.TimeFormatShort,
	})

	rec := slog.Record{
		Time:    time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		Message: "test",
		Level:   slog.LevelInfo,
	}
	h.Handle(context.Background(), rec)

	got := buf.String()
	if !strings.Contains(got, "10:30:45") {
		t.Errorf("Handle output missing time: %s", got)
	}
}

func TestPrettyHandler_Handle_WithAttributes(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewPrettyHandler(&buf, logging.Options{
		Label:   "",
		NoColor: true,
		Time:    logging.TimeFormatNone,
	})

	rec := slog.Record{
		Time:    time.Now(),
		Message: "request",
		Level:   slog.LevelInfo,
	}
	rec.AddAttrs(
		slog.String("method", "GET"),
		slog.Int("status", 200),
		slog.Bool("success", true),
	)
	h.Handle(context.Background(), rec)

	got := buf.String()
	if !strings.Contains(got, "method=GET") {
		t.Errorf("Handle output missing method attr: %s", got)
	}
	if !strings.Contains(got, "status=200") {
		t.Errorf("Handle output missing status attr: %s", got)
	}
	if !strings.Contains(got, "success=true") {
		t.Errorf("Handle output missing success attr: %s", got)
	}
}

func TestPrettyHandler_Handle_AttributesSorted(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewPrettyHandler(&buf, logging.Options{
		Label:   "",
		NoColor: true,
		Time:    logging.TimeFormatNone,
	})

	rec := slog.Record{
		Time:    time.Now(),
		Message: "test",
		Level:   slog.LevelInfo,
	}
	// Add in non-alphabetical order
	rec.AddAttrs(
		slog.String("zebra", "last"),
		slog.String("apple", "first"),
		slog.String("middle", "value"),
	)
	h.Handle(context.Background(), rec)

	got := buf.String()
	// Check alphabetical ordering: apple, middle, zebra
	appleIdx := strings.Index(got, "apple=first")
	middleIdx := strings.Index(got, "middle=value")
	zebraIdx := strings.Index(got, "zebra=last")

	if appleIdx == -1 || middleIdx == -1 || zebraIdx == -1 {
		t.Fatalf("Missing attributes in: %s", got)
	}
	if appleIdx > middleIdx || middleIdx > zebraIdx {
		t.Errorf("Attributes not sorted: %s", got)
	}
}

func TestPrettyHandler_Handle_Levels(t *testing.T) {
	tests := []struct {
		level  slog.Level
		expect string
	}{
		{slog.LevelDebug, "DEBUG"},
		{slog.LevelInfo, "INFO"},
		{slog.LevelWarn, "WARN"},
		{slog.LevelError, "ERROR"},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		h := logging.NewPrettyHandler(&buf, logging.Options{
			NoColor: true,
			Time:    logging.TimeFormatNone,
		})

		rec := slog.Record{
			Time:    time.Now(),
			Message: "msg",
			Level:   tt.level,
		}
		h.Handle(context.Background(), rec)

		got := buf.String()
		if !strings.Contains(got, tt.expect) {
			t.Errorf("Handle level %v: missing %s in %s", tt.level, tt.expect, got)
		}
	}
}

func TestPrettyHandler_Enabled(t *testing.T) {
	h := logging.NewPrettyHandler(os.Stdout, logging.Options{})
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled should return true for all levels")
	}
}

func TestPrettyHandler_WithAttrs(t *testing.T) {
	h := logging.NewPrettyHandler(&bytes.Buffer{}, logging.Options{})
	got := h.WithAttrs([]slog.Attr{slog.String("k", "v")})
	if got != h {
		t.Error("WithAttrs should return self (inline attrs)")
	}
}

func TestPrettyHandler_WithGroup(t *testing.T) {
	h := logging.NewPrettyHandler(&bytes.Buffer{}, logging.Options{})
	got := h.WithGroup("group")
	if got != h {
		t.Error("WithGroup should return self (groups not supported)")
	}
}

func TestJSONHandler_Handle_Basic(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONHandler(&buf)

	rec := slog.Record{
		Time:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Message: "hello",
		Level:   slog.LevelInfo,
	}
	h.Handle(context.Background(), rec)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Invalid JSON: %s\nBuffer: %s", err, buf.String())
	}

	if got["msg"] != "hello" {
		t.Errorf("JSON msg = %v, want hello", got["msg"])
	}
	if got["level"] != "INFO" {
		t.Errorf("JSON level = %v, want INFO", got["level"])
	}
}

func TestJSONHandler_Handle_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONHandler(&buf)

	rec := slog.Record{
		Time:    time.Now(),
		Message: "request",
		Level:   slog.LevelInfo,
	}
	rec.AddAttrs(
		slog.String("method", "POST"),
		slog.Int("count", 42),
	)
	h.Handle(context.Background(), rec)

	var got map[string]any
	json.Unmarshal(buf.Bytes(), &got)

	if got["method"] != "POST" {
		t.Errorf("JSON method = %v, want POST", got["method"])
	}
	if got["count"] != float64(42) { // JSON numbers are float64
		t.Errorf("JSON count = %v, want 42", got["count"])
	}
}

func TestJSONHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONHandler(&buf)
	hWithAttrs := h.WithAttrs([]slog.Attr{slog.String("preset", "value")})

	rec := slog.Record{
		Time:    time.Now(),
		Message: "test",
		Level:   slog.LevelInfo,
	}
	hWithAttrs.Handle(context.Background(), rec)

	var got map[string]any
	json.Unmarshal(buf.Bytes(), &got)

	// WithAttrs returns a handler; verify it works without panic
	if got["msg"] != "test" {
		t.Errorf("JSON msg = %v, want test", got["msg"])
	}
}

func TestJSONHandler_WithGroup(t *testing.T) {
	var buf bytes.Buffer
	h := logging.NewJSONHandler(&buf)

	// WithGroup should return a handler
	got := h.WithGroup("test")
	if got == nil {
		t.Error("WithGroup should return a handler")
	}

	// Verify it doesn't panic
	rec := slog.Record{
		Time:    time.Now(),
		Message: "test",
		Level:   slog.LevelInfo,
	}
	rec.AddAttrs(slog.String("key", "value"))
	got.Handle(context.Background(), rec)
}

func TestJSONHandler_Enabled(t *testing.T) {
	h := logging.NewJSONHandler(os.Stdout)
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled should return true for all levels")
	}
}

func TestLogger_ReturnsSlogLogger(t *testing.T) {
	logger := logging.Logger("[test]", logging.ColorBlue)
	if logger == nil {
		t.Error("Logger returned nil")
	}
}

func TestLoggerWithOptions_ReturnsSlogLogger(t *testing.T) {
	logger := logging.LoggerWithOptions(logging.Options{
		Label:   "[custom]",
		Color:   logging.ColorRed,
		NoColor: true,
		Time:    logging.TimeFormatNone,
	})
	if logger == nil {
		t.Error("LoggerWithOptions returned nil")
	}
}

func TestLevelColors(t *testing.T) {
	// Verify LevelColors map has entries for standard levels
	expected := []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError}
	for _, level := range expected {
		if _, ok := logging.LevelColors[level]; !ok {
			t.Errorf("LevelColors missing entry for %v", level)
		}
	}
}

func TestAgentColorPalette(t *testing.T) {
	palette := logging.AgentColorPalette
	if len(palette) < 4 {
		t.Errorf("AgentColorPalette too short: %v", palette)
	}
	// Verify all are valid ANSI color codes (30-97)
	for _, c := range palette {
		if c < 30 || c > 97 {
			t.Errorf("Invalid color in palette: %v", c)
		}
	}
}
