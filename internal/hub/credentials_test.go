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

func TestBuildCredentialEnv_InjectsFile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "my-profile")

	// Per-agent credentials.toml in agent dir.
	credsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, credsPath, map[string]credentials.CredentialProfile{
		"my-profile": {Name: "my-profile", Data: map[string]string{"api_key": "secret-key"}},
	})

	// Override ZLAW_HOME so resolveAgentDir uses our temp dir.
	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "coding"} // Dir empty → uses ZLAW_HOME/agents/coding

	envVars, err := hub.BuildCredentialEnv(entry)
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
	if injectedPath != credsPath {
		t.Errorf("expected credsPath %q, got %q", credsPath, injectedPath)
	}

	// Injected file should be readable and contain the profile.
	data, err := os.ReadFile(injectedPath)
	if err != nil {
		t.Fatalf("read injected creds file: %v", err)
	}
	var store credentials.CredentialStore
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
	if p.Data["api_key"] != "secret-key" {
		t.Errorf("api_key = %q, want %q", p.Data["api_key"], "secret-key")
	}
}

func TestBuildCredentialEnv_MissingProfile(t *testing.T) {
	tmp := t.TempDir()
	agentDir := filepath.Join(tmp, "agents", "coding")
	writeAgentTOML(t, agentDir, "missing-profile")

	// Per-agent credentials.toml with different profile.
	credsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, credsPath, map[string]credentials.CredentialProfile{
		"other-profile": {Name: "other-profile", Data: map[string]string{"api_key": "key"}},
	})

	t.Setenv("ZLAW_HOME", tmp)

	entry := config.AgentEntry{Name: "coding"}

	_, err := hub.BuildCredentialEnv(entry)
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

	envVars, err := hub.BuildCredentialEnv(entry)
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

	envVars, err := hub.BuildCredentialEnv(entry)
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

	credsPath := filepath.Join(agentDir, "credentials.toml")
	writeCredentials(t, credsPath, map[string]credentials.CredentialProfile{
		"my-profile": {Name: "my-profile", Data: map[string]string{"api_key": "s3cr3t"}},
	})

	t.Setenv("ZLAW_HOME", tmp)

	// Explicit Dir overrides the default ZLAW_HOME/agents/<name> resolution.
	entry := config.AgentEntry{Name: "coding", Dir: agentDir}

	envVars, err := hub.BuildCredentialEnv(entry)
	if err != nil {
		t.Fatalf("BuildCredentialEnv: %v", err)
	}
	if len(envVars) == 0 {
		t.Fatal("expected env vars, got none")
	}
	if envVars[0] != "ZLAW_CREDENTIALS_FILE="+credsPath {
		t.Errorf("expected %q, got %q", "ZLAW_CREDENTIALS_FILE="+credsPath, envVars[0])
	}
}
