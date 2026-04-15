package config

import (
	"os"
	"path/filepath"
)

// ZlawHome returns the root directory for all zlaw runtime data.
// It respects the ZLAW_HOME environment variable; if unset, it defaults to
// $HOME/.zlaw. The result is always an absolute path to avoid ambiguity
// when relative paths are used in config files.
func ZlawHome() string {
	v := os.Getenv("ZLAW_HOME")
	if v == "" {
		if home, err := os.UserHomeDir(); err == nil {
			return home + "/.zlaw"
		}
		return ".zlaw"
	}
	if filepath.IsAbs(v) {
		return v
	}
	// Resolve relative paths to absolute so callers don't get surprises
	// when cwd changes (e.g. hub spawning agents from a different dir).
	if abs, err := filepath.Abs(v); err == nil {
		return abs
	}
	return v
}
