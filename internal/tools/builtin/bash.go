package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
)

// Bash runs a shell command and returns stdout, stderr, and exit code.
type Bash struct{}

var bashSchema = []byte(`{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "Shell command to execute via bash -c."
    },
    "working_dir": {
      "type": "string",
      "description": "Working directory for the command. Defaults to the current working directory."
    },
    "timeout_seconds": {
      "type": "integer",
      "description": "Maximum execution time in seconds. Defaults to 30.",
      "minimum": 1,
      "maximum": 300
    }
  },
  "required": ["command"]
}`)

type bashInput struct {
	Command        string `json:"command"`
	WorkingDir     string `json:"working_dir"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (Bash) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "bash",
		Description: "Run a shell command via bash. Returns stdout, stderr, and exit code. Commands are executed with a timeout (default 30s, max 300s).",
		InputSchema: bashSchema,
	}
}

func (Bash) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var input bashInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("bash: invalid input: %w", err)
	}
	if input.Command == "" {
		return "", fmt.Errorf("bash: command is required")
	}

	timeout := input.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", input.Command)
	if input.WorkingDir != "" {
		cmd.Dir = input.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("bash: %w", err)
		}
	}

	return formatBashResult(stdout.String(), stderr.String(), exitCode), nil
}

func formatBashResult(stdout, stderr string, exitCode int) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "exit_code: %d\n", exitCode)
	if stdout != "" {
		fmt.Fprintf(&b, "stdout:\n%s", stdout)
		if stdout[len(stdout)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	if stderr != "" {
		fmt.Fprintf(&b, "stderr:\n%s", stderr)
		if stderr[len(stderr)-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
