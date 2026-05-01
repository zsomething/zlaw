package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

// resolveRunDir returns runDir if non-empty, otherwise $ZLAW_HOME/run.
func resolveRunDir(runDir string) string {
	if runDir != "" {
		return runDir
	}
	return filepath.Join(config.ZlawHome(), "run")
}

// hubPIDPath returns the path to the hub PID file within runDir.
func hubPIDPath(runDir string) string {
	return filepath.Join(runDir, "hub.pid")
}

// StartHub daemonizes the hub and starts it in the background.
// It returns immediately after spawning the hub process.
// If the hub is already running (PID file exists and process is alive),
// it returns nil without spawning a new process.
// runDir defaults to $ZLAW_HOME/run when empty.
func StartHub(ctx context.Context, configPath, runDir, externalNATSURL string, logger *slog.Logger, noColor bool) error {
	configPath = resolveConfigPath(configPath)
	runDir = resolveRunDir(runDir)

	// Check if already running.
	if pid, running := isHubRunning(runDir); running {
		fmt.Printf("hub already running (PID %d)\n", pid)
		return nil
	}

	// Ensure run dir exists.
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}

	// Remove stale control socket from a crashed hub so we can bind a fresh one.
	controlPath := hub.ControlSocketPath(runDir)
	if _, err := os.Stat(controlPath); err == nil {
		os.Remove(controlPath) //nolint:errcheck
	}

	// Find our own binary.
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	// Build args: "hub run --config <path> --run-dir <path> [--nats-url <url>]"
	args := []string{"hub", "run", "--config", configPath, "--run-dir", runDir}
	if externalNATSURL != "" {
		args = append(args, "--nats-url", externalNATSURL)
	}
	if noColor {
		args = append(args, "--no-color")
	}

	// Fork: parent spawns child that execs into the same binary with "hub run".
	// syscall.ForkExec replaces the current process image with the new binary,
	// so the parent must fork first.
	attrs := &os.ProcAttr{
		Dir:   "",
		Env:   os.Environ(),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Sys:   &syscall.SysProcAttr{},
	}

	// Setforexecexit makes the parent return immediately after fork,
	// leaving the child to continue. This is the Unix daemonize pattern.
	attrs.Sys = &syscall.SysProcAttr{
		Setsid: true, // new session
	}

	process, err := os.StartProcess(bin, append([]string{bin}, args...), attrs)
	if err != nil {
		return fmt.Errorf("fork hub process: %w", err)
	}

	// Parent writes PID and exits.
	if err := os.WriteFile(hubPIDPath(runDir), []byte(fmt.Sprintf("%d\n", process.Pid)), 0o600); err != nil {
		process.Kill() //nolint:errcheck
		return fmt.Errorf("write PID file: %w", err)
	}

	fmt.Printf("hub started (PID %d)\n", process.Pid)
	return nil
}

// RunHub starts the hub in the foreground and blocks until ctx is cancelled.
// This is the blocking equivalent of StartHub.
// runDir defaults to $ZLAW_HOME/run when empty.
func RunHub(ctx context.Context, configPath, runDir, externalNATSURL string, logger *slog.Logger, noColor bool) error {
	return runHub(ctx, resolveConfigPath(configPath), resolveRunDir(runDir), externalNATSURL, logger, noColor)
}

// StopHub reads the hub PID file and sends SIGTERM to the hub process.
// It waits up to 5 seconds for graceful shutdown, then sends SIGKILL.
// runDir defaults to $ZLAW_HOME/run when empty.
func StopHub(runDir string) error {
	runDir = resolveRunDir(runDir)
	pid, running := isHubRunning(runDir)
	if !running {
		fmt.Println("hub not running")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find hub process %d: %w", pid, err)
	}

	// Send SIGTERM for graceful shutdown.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err == os.ErrProcessDone {
			cleanupPIDFile(runDir)
			fmt.Println("hub not running")
			return nil //nolint:nilerr
		}
		return fmt.Errorf("signal hub: %w", err)
	}

	// Wait up to 5s for process to exit.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if p, err := os.FindProcess(pid); err != nil || p.Signal(syscall.Signal(0)) != nil {
			// Process is gone.
			cleanupPIDFile(runDir)
			fmt.Println("hub stopped")
			return nil //nolint:nilerr
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Graceful timeout — SIGKILL.
	proc.Kill() //nolint:errcheck
	cleanupPIDFile(runDir)
	fmt.Println("hub stopped (forced)")
	return nil
}

// isHubRunning returns the hub PID and true if a hub process is currently running.
func isHubRunning(runDir string) (pid int, running bool) {
	data, err := os.ReadFile(hubPIDPath(runDir))
	if err != nil {
		return 0, false
	}
	var p int
	for _, c := range string(data) {
		if c < '0' || c > '9' {
			break
		}
		p = p*10 + int(c-'0')
	}
	if p == 0 {
		return 0, false
	}
	proc, err := os.FindProcess(p)
	if err != nil {
		return 0, false
	}
	// Signal 0 checks liveness without sending a signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is gone — stale PID file.
		cleanupPIDFile(runDir)
		return 0, false
	}
	return p, true
}

func cleanupPIDFile(runDir string) {
	os.Remove(hubPIDPath(runDir)) //nolint:errcheck
}

// resolveConfigPath returns configPath if non-empty, otherwise the default.
func resolveConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	return config.DefaultHubConfigPath()
}
