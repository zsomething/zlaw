package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chickenzord/zlaw/internal/config"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Basic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "test-agent"
description = "A test agent"

[llm]
backend = "anthropic"
model = "claude-haiku-4-5-20251001"
api_key_env = "TEST_API_KEY"
max_tokens = 1024
timeout_sec = 30

[tools]
allowed = ["echo", "current-time"]

[adapter]
type = "cli"
`)
	writeFile(t, dir, "SOUL.md", "You are a helpful assistant.")
	writeFile(t, dir, "IDENTITY.md", "My name is Test.")

	loader, err := config.NewLoader(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, p, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Agent.Name != "test-agent" {
		t.Errorf("agent.name = %q, want %q", cfg.Agent.Name, "test-agent")
	}
	if cfg.LLM.Backend != "anthropic" {
		t.Errorf("llm.backend = %q, want %q", cfg.LLM.Backend, "anthropic")
	}
	if len(cfg.Tools.Allowed) != 2 {
		t.Errorf("tools.allowed len = %d, want 2", len(cfg.Tools.Allowed))
	}
	if cfg.Adapter.Type != "cli" {
		t.Errorf("adapter.type = %q, want %q", cfg.Adapter.Type, "cli")
	}
	if p.Soul != "You are a helpful assistant." {
		t.Errorf("soul = %q", p.Soul)
	}
	if p.Identity != "My name is Test." {
		t.Errorf("identity = %q", p.Identity)
	}
}

func TestLoad_MissingPersonalityFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "minimal"
[llm]
api_key_env = "KEY"
[tools]
[adapter]
type = "cli"
`)
	// No SOUL.md or IDENTITY.md

	loader, err := config.NewLoader(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, p, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if p.Soul != "" {
		t.Errorf("expected empty soul, got %q", p.Soul)
	}
	if p.Identity != "" {
		t.Errorf("expected empty identity, got %q", p.Identity)
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MY_MODEL", "claude-opus-4-6")
	writeFile(t, dir, "agent.toml", `
[agent]
name = "env-test"
[llm]
backend = "anthropic"
model = "${MY_MODEL}"
api_key_env = "KEY"
[tools]
[adapter]
type = "cli"
`)

	loader, err := config.NewLoader(dir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "claude-opus-4-6" {
		t.Errorf("llm.model = %q, want %q", cfg.LLM.Model, "claude-opus-4-6")
	}
}

func TestAPIKey_ResolvedFromEnv(t *testing.T) {
	t.Setenv("AGENT_API_KEY", "sk-test-123")
	cfg := config.LLMConfig{APIKeyEnv: "AGENT_API_KEY"}
	key, err := cfg.APIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "sk-test-123" {
		t.Errorf("key = %q, want %q", key, "sk-test-123")
	}
}

func TestAPIKey_MissingEnv(t *testing.T) {
	cfg := config.LLMConfig{APIKeyEnv: "DEFINITELY_NOT_SET_XYZ"}
	os.Unsetenv("DEFINITELY_NOT_SET_XYZ")
	_, err := cfg.APIKey()
	if err == nil {
		t.Error("expected error for unset env var")
	}
}
