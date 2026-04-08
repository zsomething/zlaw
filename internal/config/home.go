package config

import "os"

// ZlawHome returns the root directory for all zlaw runtime data.
// It respects the ZLAW_HOME environment variable; if unset, it defaults to
// the current working directory.
func ZlawHome() string {
	if v := os.Getenv("ZLAW_HOME"); v != "" {
		return v
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
