package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsomething/zlaw/internal/config"
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
id = "test-agent"
name = "Test Agent"
description = "A test agent"

[llm]
backend = "anthropic"
model = "claude-haiku-4-5-20251001"
auth_profile = "anthropic-default"
max_tokens = 1024
timeout_sec = 30

[tools]
allowed = ["echo", "current-time"]

[[adapter]]
type = "cli"
`)
	writeFile(t, dir, "SOUL.md", "You are a helpful assistant.")
	writeFile(t, dir, "IDENTITY.md", "My name is Test.")

	loader, err := config.NewLoader(dir, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, p, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Agent.ID != "test-agent" {
		t.Errorf("agent.id = %q, want %q", cfg.Agent.ID, "test-agent")
	}
	if cfg.Agent.Name != "Test Agent" {
		t.Errorf("agent.name = %q, want %q", cfg.Agent.Name, "Test Agent")
	}
	if cfg.Agent.DisplayName() != "Test Agent" {
		t.Errorf("DisplayName() = %q, want %q", cfg.Agent.DisplayName(), "Test Agent")
	}
	if cfg.LLM.Backend != "anthropic" {
		t.Errorf("llm.backend = %q, want %q", cfg.LLM.Backend, "anthropic")
	}
	if cfg.LLM.AuthProfile != "anthropic-default" {
		t.Errorf("llm.auth_profile = %q, want %q", cfg.LLM.AuthProfile, "anthropic-default")
	}
	if len(cfg.Tools.Allowed) != 2 {
		t.Errorf("tools.allowed len = %d, want 2", len(cfg.Tools.Allowed))
	}
	if len(cfg.Adapter) != 1 || cfg.Adapter[0].Type != "cli" {
		t.Errorf("adapter = %v, want [cli]", cfg.Adapter)
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
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)
	// No SOUL.md or IDENTITY.md

	loader, err := config.NewLoader(dir, "", nil, nil)
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

func TestLoad_RuntimeOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "runtime-test"
[llm]
backend = "anthropic"
model = "claude-haiku-4-5-20251001"
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)
	writeFile(t, dir, "runtime.toml", `
[llm]
model = "claude-opus-4-6"
`)

	loader, err := config.NewLoader(dir, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "claude-opus-4-6" {
		t.Errorf("llm.model = %q, want %q (runtime override not applied)", cfg.LLM.Model, "claude-opus-4-6")
	}
}

func TestLoad_MissingRuntimeTOML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "no-runtime"
[llm]
model = "claude-haiku-4-5-20251001"
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)
	// No runtime.toml — should succeed with static model.

	loader, err := config.NewLoader(dir, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("llm.model = %q, want static value %q", cfg.LLM.Model, "claude-haiku-4-5-20251001")
	}
}

func TestWriteRuntimeField_AndReload(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "write-test"
[llm]
model = "claude-haiku-4-5-20251001"
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)

	var gotModel string
	onChange := func(cfg config.AgentConfig, _ config.Personality) {
		gotModel = cfg.LLM.Model
	}
	loader, err := config.NewLoader(dir, "", onChange, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if err := loader.WriteRuntimeField("llm.model", "claude-opus-4-6"); err != nil {
		t.Fatalf("WriteRuntimeField: %v", err)
	}
	if err := loader.ReloadRuntime(); err != nil {
		t.Fatalf("ReloadRuntime: %v", err)
	}
	if gotModel != "claude-opus-4-6" {
		t.Errorf("onChange got model %q, want %q", gotModel, "claude-opus-4-6")
	}
}

func TestWriteRuntimeField_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "agent.toml", `
[agent]
name = "invalid-key-test"
[llm]
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)
	loader, err := config.NewLoader(dir, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := loader.Load(); err != nil {
		t.Fatal(err)
	}
	if err := loader.WriteRuntimeField("llm.backend", "openai"); err == nil {
		t.Error("expected error for non-configurable field, got nil")
	}
}

func TestAgentMeta_DisplayName(t *testing.T) {
	tests := []struct {
		meta config.AgentMeta
		want string
	}{
		{config.AgentMeta{ID: "manager", Name: "Rin"}, "Rin"},
		{config.AgentMeta{ID: "manager", Name: ""}, "manager"},
		{config.AgentMeta{ID: "", Name: ""}, ""},
	}
	for _, tc := range tests {
		got := tc.meta.DisplayName()
		if got != tc.want {
			t.Errorf("DisplayName() for {ID:%q Name:%q} = %q, want %q", tc.meta.ID, tc.meta.Name, got, tc.want)
		}
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
auth_profile = "default"
[tools]
[[adapter]]
type = "cli"
`)

	loader, err := config.NewLoader(dir, "", nil, nil)
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
