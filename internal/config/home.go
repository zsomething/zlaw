package config

import (
	"os"
	"path/filepath"
)

// ZlawHome returns the root directory for all zlaw runtime data.
// It respects the ZLAW_HOME environment variable; if unset, it defaults to
// $HOME/.zlaw.
func ZlawHome() string {
	if v := os.Getenv("ZLAW_HOME"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".zlaw")
	}
	return ".zlaw"
}
