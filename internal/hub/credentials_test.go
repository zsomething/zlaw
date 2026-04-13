package hub_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/llm/auth"
)

// writeAgentTOML writes a minimal agent.toml with the given auth_profile to dir.
func writeAgentTOML(t *testing.T, dir, authProfile string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "[agent]\nname = \"test\"\n"
	if authProfile != "" {
		content += "[llm]\nauth_profile = \"" + authProfile + "\"\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write agent.toml: %v", err)
	}
}

// writeCredentials writes a credentials.toml with the given apikey profiles to path.
func writeCredentials(t *testing.T, path string, profiles map[string]string) {
	t.Helper()
	store := auth.CredentialStore{Profiles: make(map[string]auth.CredentialProfile, len(profiles))}
	for name, key := range profiles {
		store.Profiles[name] = auth.CredentialProfile{Type: auth.ProfileTypeAPIKey, Key: key}
	}
	if err := auth.SaveStore(path, store); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
}

func TestBuildCredentialEnv_InjectsFile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "my-profile")

	credsPath := filepath.Join(tmp, "credentials.toml")
	writeCredentials(t, credsPath, map[string]string{"my-profile": "secret-key"})

	// Override ZLAW_HOME so resolveAgentDir and run-dir use our temp dir.
	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "coding"} // Dir empty → uses ZLAW_HOME/agents/coding

	envVars, err := hub.BuildCredentialEnv(entry, credsPath)
	if err != nil {
		t.Fatalf("BuildCredentialEnv: %v", err)
	}
	if len(envVars) == 0 {
		t.Fatal("expected env vars, got none")
	}

	// Find ZLAW_CREDENTIALS_FILE.
	const prefix = "ZLAW_CREDENTIALS_FILE="
	var injectedPath string
	for _, kv := range envVars {
		if len(kv) > len(prefix) && kv[:len(prefix)] == prefix {
			injectedPath = kv[len(prefix):]
		}
	}
	if injectedPath == "" {
		t.Fatalf("ZLAW_CREDENTIALS_FILE not in env vars: %v", envVars)
	}

	// Injected file should be readable and contain only the needed profile.
	data, err := os.ReadFile(injectedPath)
	if err != nil {
		t.Fatalf("read injected creds file: %v", err)
	}
	var store auth.CredentialStore
	if _, err := toml.Decode(string(data), &store); err != nil {
		t.Fatalf("parse injected creds file: %v", err)
	}
	if len(store.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(store.Profiles))
	}
	p, ok := store.Profiles["my-profile"]
	if !ok {
		t.Error("my-profile not in injected credentials")
	}
	if p.Key != "secret-key" {
		t.Errorf("key = %q, want %q", p.Key, "secret-key")
	}
}

func TestBuildCredentialEnv_MissingProfile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "missing-profile")

	credsPath := filepath.Join(tmp, "credentials.toml")
	writeCredentials(t, credsPath, map[string]string{"other-profile": "key"})

	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "coding"}

	_, err := hub.BuildCredentialEnv(entry, credsPath)
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestBuildCredentialEnv_NoAuthProfile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "noauth")
	writeAgentTOML(t, agentDir, "") // empty auth_profile

	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "noauth"}

	envVars, err := hub.BuildCredentialEnv(entry, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(envVars) != 0 {
		t.Errorf("expected no env vars, got %v", envVars)
	}
}

func TestBuildCredentialEnv_MissingAgentTOML(t *testing.T) {
	tmp := t.TempDir()
	// Do not write agent.toml at all.
	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "ghost"}

	envVars, err := hub.BuildCredentialEnv(entry, "")
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

	credsPath := filepath.Join(tmp, "credentials.toml")
	writeCredentials(t, credsPath, map[string]string{"my-profile": "s3cr3t"})

	t.Setenv("ZLAW_HOME", tmp)

	// Explicit Dir overrides the default ZLAW_HOME/agents/<name> resolution.
	entry := config.AgentEntry{Name: "coding", Dir: agentDir}

	envVars, err := hub.BuildCredentialEnv(entry, credsPath)
	if err != nil {
		t.Fatalf("BuildCredentialEnv: %v", err)
	}
	if len(envVars) == 0 {
		t.Fatal("expected env vars, got none")
	}
}
