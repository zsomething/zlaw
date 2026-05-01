package envfilter

import (
	"slices"
	"testing"
)

func TestFilter_RemovesCredentialVars(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"HOME=/home/user",
		"MINIMAX_API_KEY=secret",
		"ANTHROPIC_API_KEY=sk-ant-secret",
		"TELEGRAM_BOT_TOKEN=12345",
		"USER=test",
	}

	filtered := Filter(env)

	// Essential vars should be present.
	if !slices.Contains(filtered, "PATH=/usr/bin") {
		t.Error("PATH should be preserved")
	}
	if !slices.Contains(filtered, "HOME=/home/user") {
		t.Error("HOME should be preserved")
	}
	if !slices.Contains(filtered, "USER=test") {
		t.Error("USER should be preserved")
	}

	// Credential vars should be removed.
	if slices.Contains(filtered, "MINIMAX_API_KEY=secret") {
		t.Error("MINIMAX_API_KEY should be removed")
	}
	if slices.Contains(filtered, "ANTHROPIC_API_KEY=sk-ant-secret") {
		t.Error("ANTHROPIC_API_KEY should be removed")
	}
	if slices.Contains(filtered, "TELEGRAM_BOT_TOKEN=12345") {
		t.Error("TELEGRAM_BOT_TOKEN should be removed")
	}
}

func TestFilter_RemovesCredentialPrefixes(t *testing.T) {
	env := []string{
		"MINIMAX_FOO=bar",
		"ANTHROPIC_BAR=baz",
		"OPENAI_KEY=key",
		"FIZZY_API_KEY=fizzy-key",
		"ZLAW_CREDENTIALS_FILE=/path/to/creds",
		"ZLAW_NATS_CREDS=token",
		"HOME=/home/user",
	}

	filtered := Filter(env)

	// Should only have HOME.
	if len(filtered) != 1 {
		t.Errorf("expected 1 var, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "HOME=/home/user" {
		t.Errorf("expected HOME, got %v", filtered)
	}
}

func TestFilter_PreservesNonCredentialVars(t *testing.T) {
	env := []string{
		"MY_CUSTOM_VAR=value",
		"SOME_SERVICE_URL=https://example.com",
		"DEBUG=true",
		"HOME=/home/user",
	}

	filtered := Filter(env)

	// All non-credential vars should be preserved.
	if len(filtered) != 4 {
		t.Errorf("expected 4 vars, got %d: %v", len(filtered), filtered)
	}
}

func TestFilter_HandlesEmptyEnv(t *testing.T) {
	filtered := Filter([]string{})
	if len(filtered) != 0 {
		t.Errorf("expected 0 vars, got %d", len(filtered))
	}
}

func TestFilter_HandlesMalformedEntries(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"NO_EQUALS",
		"MINIMAX_KEY=secret",
		"",
		"HOME=/home/user",
	}

	filtered := Filter(env)

	// Should handle malformed entries gracefully.
	if len(filtered) != 2 {
		t.Errorf("expected 2 vars, got %d: %v", len(filtered), filtered)
	}
}
