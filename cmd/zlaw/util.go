package main

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/zsomething/zlaw/internal/llm/auth"
)

func logLevel() slog.Level {
	if os.Getenv("ZLAW_LOG_LEVEL") == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// prompt writes label to stdout and reads a line from stdin.
// If sensitive is true, it attempts to disable terminal echo (best-effort).
func prompt(label string, sensitive bool) (string, error) {
	fmt.Print(label)

	if sensitive {
		_ = runStty("-echo")
		defer func() { _ = runStty("echo") }()
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("unexpected EOF")
	}

	if sensitive {
		fmt.Println()
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func runStty(arg string) error {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("stty", arg)
	cmd.Stdin = f
	return cmd.Run()
}

func profileDetails(p auth.CredentialProfile) string {
	switch p.Type {
	case auth.ProfileTypeAPIKey:
		if len(p.Key) > 8 {
			return "key=" + p.Key[:4] + "..." + p.Key[len(p.Key)-4:]
		}
		return "key=***"
	case auth.ProfileTypeOAuth2:
		parts := []string{"token_url=" + p.TokenURL, "client_id=" + p.ClientID}
		if p.Scope != "" {
			parts = append(parts, "scope="+p.Scope)
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}
