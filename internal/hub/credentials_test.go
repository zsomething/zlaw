package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
	"github.com/zsomething/zlaw/internal/hub"
)

// writeAgentTOML writes a minimal agent.toml with the given auth_profile to dir.
func writeAgentTOML(t *testing.T, dir, authProfile string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "[agent]\nid = \"test\"\n"
	if authProfile != "" {
		content += "[llm]\nauth_profile = \"" + authProfile + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write agent.toml: %v", err)
	}
}

// writeCredentials writes a credentials.toml with the given profiles to path.
func writeCredentials(t *testing.T, path string, profiles map[string]credentials.CredentialProfile) {
	t.Helper()
	store := credentials.CredentialStore{Profiles: make(map[string]credentials.CredentialProfile, len(profiles))}
	for name, p := range profiles {
		store.Profiles[name] = p
	}
	if err := credentials.SaveStore(path, store); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}

func TestBuildCredentialEnv_WritesActiveFile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "my-profile")

	// User-maintained source credentials file — never touched by hub.
	sourceCredsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, sourceCredsPath, map[string]credentials.CredentialProfile{
		"my-profile":    {Name: "my-profile", Data: map[string]string{"api_key": "secret-key"}},
		"extra-profile": {Name: "extra-profile", Data: map[string]string{"api_key": "extra"}},
	})

	// Snapshot source so we can verify it survives.
	srcContent, _ := os.ReadFile(sourceCredsPath)

	t.Setenv("ZLAW_HOME", tmp)

	envVars, err := hub.BuildCredentialEnv(config.AgentEntry{ID: "coding"})
	if err != nil {
		t.Fatalf("BuildCredentialEnv: %v", err)
	}
	if len(envVars) == 0 {
		t.Fatal("expected env vars, got none")
	}

	// Active file goes to run/credentials/<name>.toml.
	const prefix = "ZLAW_CREDENTIALS_FILE="
	runDir := filepath.Join(tmp, "run")
	activeCredsPath := filepath.Join(runDir, "credentials", "coding.toml")
	var injectedPath string
	for _, kv := range envVars {
		if len(kv) > len(prefix) && kv[:len(prefix)] == prefix {
			injectedPath = kv[len(prefix):]
		}
	}
	if injectedPath != activeCredsPath {
		t.Errorf("expected ZLAW_CREDENTIALS_FILE=%q, got %q", activeCredsPath, injectedPath)
	}

	// Source file must be untouched (user-maintained).
	afterSrc, _ := os.ReadFile(sourceCredsPath)
	if string(afterSrc) != string(srcContent) {
		t.Error("source credentials.toml was modified — should be untouched")
	}

	// Active file should be readable and contain only the referenced profile.
	data, err := os.ReadFile(activeCredsPath)
	if err != nil {
		t.Fatalf("read active credentials: %v", err)
	}
	var store credentials.CredentialStore
	if _, err := toml.Decode(string(data), &store); err != nil {
		t.Fatalf("parse active credentials: %v", err)
	}
	if len(store.Profiles) != 1 {
		t.Errorf("expected 1 profile in active file, got %d", len(store.Profiles))
	}
	p, ok := store.Profiles["my-profile"]
	if !ok {
		t.Error("my-profile not in active credentials")
	}
	if p.Data["api_key"] != "secret-key" {
		t.Errorf("api_key = %q, want %q", p.Data["api_key"], "secret-key")
	}
}

func TestBuildCredentialEnv_MissingProfile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "missing-profile")

	sourceCredsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, sourceCredsPath, map[string]credentials.CredentialProfile{
		"other-profile": {Name: "other-profile", Data: map[string]string{"api_key": "key"}},
	})

	t.Setenv("ZLAW_HOME", tmp)

	_, err := hub.BuildCredentialEnv(config.AgentEntry{ID: "coding"})
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestBuildCredentialEnv_NoAuthProfile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "noauth")
	writeAgentTOML(t, agentDir, "") // empty auth_profile

	t.Setenv("ZLAW_HOME", tmp)

	envVars, err := hub.BuildCredentialEnv(config.AgentEntry{ID: "noauth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envVars) != 0 {
		t.Errorf("expected no env vars, got %v", envVars)
	}
}

func TestBuildCredentialEnv_MissingAgentTOML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("ZLAW_HOME", tmp)

	envVars, err := hub.BuildCredentialEnv(config.AgentEntry{ID: "ghost"})
	if err != nil {
		t.Fatalf("missing agent.toml should be a no-op, got err: %v", err)
	}
	if len(envVars) != 0 {
		t.Errorf("expected no env vars for missing agent.toml, got %v", envVars)
	}
}

func TestBuildCredentialEnv_ExplicitDir(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "custom-dir")
	writeAgentTOML(t, agentDir, "my-profile")

	sourceCredsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, sourceCredsPath, map[string]credentials.CredentialProfile{
		"my-profile": {Name: "my-profile", Data: map[string]string{"api_key": "s3cr3t"}},
	})

	t.Setenv("ZLAW_HOME", tmp)

	runDir := filepath.Join(tmp, "run")
	activeCredsPath := filepath.Join(runDir, "credentials", "coding.toml")
	envVars, err := hub.BuildCredentialEnv(config.AgentEntry{ID: "coding", Dir: agentDir})
	if err != nil {
		t.Fatalf("BuildCredentialEnv: %v", err)
	}
	if len(envVars) == 0 {
		t.Fatal("expected env vars, got none")
	}
	if envVars[0] != "ZLAW_CREDENTIALS_FILE="+activeCredsPath {
		t.Errorf("expected %q, got %q", "ZLAW_CREDENTIALS_FILE="+activeCredsPath, envVars[0])
	}
}
