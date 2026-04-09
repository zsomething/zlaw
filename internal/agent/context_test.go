package agent_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chickenzord/zlaw/internal/agent"
)

func TestBuildPrefill_Empty(t *testing.T) {
	out, err := agent.BuildPrefill("/any", nil)
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("empty sources should return empty string, got: %q", out)
	}
}

func TestBuildPrefill_CWD(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"cwd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cwd:") {
		t.Fatalf("expected cwd: in output, got: %q", out)
	}
}

func TestBuildPrefill_Datetime(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"datetime"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "datetime:") {
		t.Fatalf("expected datetime: in output, got: %q", out)
	}
}

func TestBuildPrefill_File(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("test soul"), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := agent.BuildPrefill(dir, []string{"file:SOUL.md"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "test soul") {
		t.Fatalf("expected file contents in output, got: %q", out)
	}
	if !strings.Contains(out, "file:SOUL.md") {
		t.Fatalf("expected file label in output, got: %q", out)
	}
}

func TestBuildPrefill_UnknownSource(t *testing.T) {
	_, err := agent.BuildPrefill("/any", []string{"bogus"})
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestBuildPrefill_MultipleSourcesHeader(t *testing.T) {
	out, err := agent.BuildPrefill("/any", []string{"cwd", "datetime"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "[Session context]") {
		t.Fatalf("expected [Session context] header, got: %q", out)
	}
}
