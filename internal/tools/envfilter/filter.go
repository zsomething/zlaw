// Package envfilter provides environment variable filtering for subprocess security.
// It prevents credential leakage by removing sensitive env vars before exec.
package envfilter

import (
	"strings"
)

// CredentialPrefixes are prefixes of env var names that contain secrets.
// These are filtered from subprocess environment.
var CredentialPrefixes = []string{
	"MINIMAX_",
	"ANTHROPIC_",
	"TELEGRAM_",
	"FIZZY_",
	"OPENAI_",
	"ZLAW_CREDENTIALS_FILE",
	"ZLAW_NATS_CREDS",
}

// EssentialVars are the env vars that subprocesses are allowed to inherit.
// These are runtime/environment vars needed for basic operation.
var EssentialVars = []string{
	"ZLAW_AGENT_HOME",
	"ZLAW_AGENT",
	"ZLAW_NATS_URL",
	"ZLAW_LOG_LEVEL",
	"ZLAW_LOG_FORMAT",
	"ZLAW_NO_COLOR",
	"PATH",
	"HOME",
	"USER",
	"TERM",
	"PWD",
	"LANG",
	"LC_ALL",
	"TZ",
}

// Filter returns env with credential vars removed, essential vars kept.
// It preserves any env var not in CredentialPrefixes and either in EssentialVars
// or not explicitly excluded.
func Filter(env []string) []string {
	// Build set of credential prefixes.
	credPrefixes := make(map[string]bool)
	for _, p := range CredentialPrefixes {
		credPrefixes[p] = true
	}

	// Build set of essential vars.
	essential := make(map[string]bool)
	for _, v := range EssentialVars {
		essential[v] = true
	}

	var filtered []string
	for _, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			continue
		}
		key := e[:idx]

		// Skip if key matches credential prefix.
		skip := false
		for prefix := range credPrefixes {
			if strings.HasPrefix(key, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		filtered = append(filtered, e)
	}

	return filtered
}
