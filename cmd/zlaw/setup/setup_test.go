package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsomething/zlaw/internal/config"
)

func TestBootstrapConfig(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	// Test BootstrapConfig.
	cfg := config.BootstrapConfig{
		Home: tmpDir,
	}

	// Test Exists() before create.
	if cfg.Exists() {
		t.Error("BootstrapConfig.Exists() should return false before CreateZlawHome()")
	}

	// Create zlaw home.
	if err := cfg.CreateZlawHome(); err != nil {
		t.Fatalf("BootstrapConfig.CreateZlawHome() failed: %v", err)
	}

	// Test Exists() after create.
	if !cfg.Exists() {
		t.Error("BootstrapConfig.Exists() should return true after CreateZlawHome()")
	}

	// Verify files created.
	zlawPath := filepath.Join(tmpDir, "zlaw.toml")
	if _, err := os.Stat(zlawPath); os.IsNotExist(err) {
		t.Error("zlaw.toml not created")
	}

	secretsPath := filepath.Join(tmpDir, "secrets.toml")
	if _, err := os.Stat(secretsPath); os.IsNotExist(err) {
		t.Error("secrets.toml not created")
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		t.Error("agents/ directory not created")
	}
}

func TestSetupAgentConfig(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	// Create zlaw home first.
	bootstrapCfg := config.BootstrapConfig{
		Home: tmpDir,
	}
	if err := bootstrapCfg.CreateZlawHome(); err != nil {
		t.Fatalf("BootstrapConfig.CreateZlawHome() failed: %v", err)
	}

	// Test SetupAgentConfig.
	agentCfg := config.SetupAgentConfig{
		ID:   "test-agent",
		Home: tmpDir,
	}

	// Test AgentDir().
	expectedDir := filepath.Join(tmpDir, "agents", "test-agent")
	if agentCfg.AgentDir() != expectedDir {
		t.Errorf("SetupAgentConfig.AgentDir() = %q, want %q", agentCfg.AgentDir(), expectedDir)
	}

	// Test Exists() before create.
	if agentCfg.Exists() {
		t.Error("SetupAgentConfig.Exists() should return false before CreateAgent()")
	}

	// Create agent.
	if err := agentCfg.CreateAgent(); err != nil {
		t.Fatalf("SetupAgentConfig.CreateAgent() failed: %v", err)
	}

	// Test Exists() after create.
	if !agentCfg.Exists() {
		t.Error("SetupAgentConfig.Exists() should return true after CreateAgent()")
	}

	// Verify files created.
	soulPath := filepath.Join(expectedDir, "SOUL.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		t.Error("SOUL.md not created")
	}

	identityPath := filepath.Join(expectedDir, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		t.Error("IDENTITY.md not created")
	}

	workspacePath := filepath.Join(expectedDir, "workspace")
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		t.Error("workspace/ directory not created")
	}

	skillsPath := filepath.Join(expectedDir, "skills")
	if _, err := os.Stat(skillsPath); os.IsNotExist(err) {
		t.Error("skills/ directory not created")
	}
}

func TestSecretsOperations(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	// Add a secret.
	secretName := "TEST_API_KEY"
	secretValue := "test-value-123"
	if err := config.AddSecretTo(secretName, secretValue, tmpDir); err != nil {
		t.Fatalf("AddSecretTo() failed: %v", err)
	}

	// List secrets.
	secretsPath := filepath.Join(tmpDir, "secrets.toml")
	secrets := config.LoadSecretsFrom(secretsPath)
	if len(secrets) != 1 {
		t.Errorf("LoadSecretsFrom() returned %d secrets, want 1", len(secrets))
	}

	// Verify secret value.
	if secrets[secretName] != secretValue {
		t.Errorf("Secret %q = %q, want %q", secretName, secrets[secretName], secretValue)
	}

	// Remove secret.
	if err := config.RemoveSecretFrom(secretName, tmpDir); err != nil {
		t.Fatalf("RemoveSecretFrom() failed: %v", err)
	}

	// Verify removed.
	secrets = config.LoadSecretsFrom(secretsPath)
	if len(secrets) != 0 {
		t.Errorf("After RemoveSecretFrom(), got %d secrets, want 0", len(secrets))
	}
}
