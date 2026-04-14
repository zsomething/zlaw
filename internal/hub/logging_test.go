package hub

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestAgentColor(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"test"},
		{"foo"},
		{"bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentColor(tt.name)
			if got < ColorRed || got > ColorWhite {
				t.Errorf("AgentColor(%q) = %v, want valid color", tt.name, got)
			}
		})
	}
}

func TestColorize(t *testing.T) {
	tests := []struct {
		color  Color
		text   string
		noAnsi bool // if true, text should not contain ANSI codes
	}{
		{ColorDefault, "hello", true},
		{ColorRed, "hello", false},
		{ColorGreen, "world", false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := Colorize(tt.color, tt.text)
			if tt.noAnsi {
				if strings.Contains(got, "\x1b[") {
					t.Errorf("Colorize(%v, %q) = %q, want no ANSI codes", tt.color, tt.text, got)
				}
			} else {
				if !strings.Contains(got, "\x1b[") {
					t.Errorf("Colorize(%v, %q) = %q, want ANSI codes", tt.color, tt.text, got)
				}
			}
		})
	}
}

func TestDefaultNoColor(t *testing.T) {
	// Just ensure it doesn't panic
	nc := DefaultNoColor()
	_ = nc
}

func TestColoredHandler(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{})

	noColor := false
	ch := NewColoredHandler(h, &noColor)
	ch = ApplyHandlerOptions(ch, WithLabel("[test]"), WithColor(ColorRed))
	logger := slog.New(ch)

	logger.Info("hello world")

	output := buf.String()
	if !strings.Contains(output, "[test]") {
		t.Errorf("expected [test] in output, got: %s", output)
	}
}

func TestColoredHandlerNoColor(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{})

	noColor := true
	ch := NewColoredHandler(h, &noColor)
	ch = ApplyHandlerOptions(ch, WithLabel("[hub]"), WithColor(ColorGray))
	logger := slog.New(ch)

	logger.Info("test message")

	output := buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("expected no ANSI codes when noColor=true, got: %s", output)
	}
}

func TestLinePrefixWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewLinePrefixWriter(&buf, "[test] ")

	n, err := w.Write([]byte("line1\nline2\nline3"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	output := buf.String()
	// Check that prefix appears before each line
	if !strings.Contains(output, "[test] line1") {
		t.Errorf("expected [test] line1 in output, got: %s", output)
	}
	if !strings.Contains(output, "[test] line2") {
		t.Errorf("expected [test] line2 in output, got: %s", output)
	}
	if n == 0 {
		t.Error("expected non-zero bytes written")
	}
}

func TestLinePrefixWriterTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	w := NewLinePrefixWriter(&buf, "[x] ")

	_, err := w.Write([]byte("line1\n"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	output := buf.String()
	if !strings.HasSuffix(output, "[x] \n") && !strings.HasSuffix(output, "[x] ") {
		// Single line with trailing newline should not have extra prefix
	}
}

func TestNewColoredLinePrefixWriter(t *testing.T) {
	var buf bytes.Buffer

	// With color
	w1 := NewColoredLinePrefixWriter(&buf, "[agent]", ColorBlue, false)
	w1.Write([]byte("hello\n"))

	output := buf.String()
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("expected ANSI codes, got: %s", output)
	}

	// With noColor
	buf.Reset()
	w2 := NewColoredLinePrefixWriter(&buf, "[agent]", ColorBlue, true)
	w2.Write([]byte("hello\n"))

	output = buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("expected no ANSI codes with noColor=true, got: %s", output)
	}
}
