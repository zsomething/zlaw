package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/zsomething/zlaw/internal/config"
)

// setupHubTOML writes a minimal zlaw.toml to tmp/.zlaw/zlaw.toml.
func setupHubTOML(t *testing.T, content string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".zlaw")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zlaw.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write zlaw.toml: %v", err)
	}
	t.Setenv("ZLAW_HOME", t.TempDir()+"/.zlaw")
}

func TestRemoveAgent(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "test"

[[agents]]
name = "alice"
dir = "agents/alice"

[[agents]]
name = "bob"
dir = "agents/bob"

[[agents]]
name = "carol"
dir = "agents/carol"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}

	// Remove "bob" (middle of list).
	if err := cfg.RemoveAgent("bob"); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}

	// Reload and verify.
	cfg2, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(cfg2.Agents) != 2 {
		t.Errorf("Agents len = %d, want 2", len(cfg2.Agents))
	}
	names := make([]string, len(cfg2.Agents))
	for i, a := range cfg2.Agents {
		names[i] = a.Name
	}
	if names[0] != "alice" || names[1] != "carol" {
		t.Errorf("names = %v, want [alice carol]", names)
	}
}

func TestRemoveAgent_NotFound(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "test"

[[agents]]
name = "alice"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}

	err = cfg.RemoveAgent("ghost")
	// RemoveAgent silently succeeds for unknown agents (no-op removal).
	if err != nil {
		t.Errorf("unexpected error for non-existent agent: %v", err)
	}
	// Verify no agents were removed.
	cfg2, _ := config.LoadHubConfig(configPath)
	if len(cfg2.Agents) != 1 {
		t.Errorf("Agents len = %d, want 1 (ghost was not in list anyway)", len(cfg2.Agents))
	}
}

func TestRemoveAgent_LastAgent(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "solo"

[[agents]]
name = "only"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}
	if err := cfg.RemoveAgent("only"); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}

	cfg2, _ := config.LoadHubConfig(configPath)
	if len(cfg2.Agents) != 0 {
		t.Errorf("Agents len = %d, want 0", len(cfg2.Agents))
	}
}

func TestAddAgent(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "test"

[[agents]]
name = "alice"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}

	entry := config.AgentEntry{
		Name:      "bob",
		Dir:       "agents/bob",
		Workspace: "workspaces/bob",
	}
	if err := cfg.AddAgent(entry); err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	cfg2, _ := config.LoadHubConfig(configPath)
	if len(cfg2.Agents) != 2 {
		t.Fatalf("Agents len = %d, want 2", len(cfg2.Agents))
	}
	if cfg2.Agents[1].Name != "bob" {
		t.Errorf("Agents[1].Name = %q, want %q", cfg2.Agents[1].Name, "bob")
	}
	if cfg2.Agents[1].Workspace != "workspaces/bob" {
		t.Errorf("Agents[1].Workspace = %q, want %q", cfg2.Agents[1].Workspace, "workspaces/bob")
	}
}

func TestAddAgent_NoExistingAgentsSection(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "alone"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}

	if err := cfg.AddAgent(config.AgentEntry{Name: "solo"}); err != nil {
		t.Fatalf("AddAgent: %v", err)
	}

	cfg2, _ := config.LoadHubConfig(configPath)
	if len(cfg2.Agents) != 1 {
		t.Fatalf("Agents len = %d, want 1", len(cfg2.Agents))
	}
}

func TestFindAgent(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "test"

[[agents]]
name = "alice"

[[agents]]
name = "bob"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, _ := config.LoadHubConfig(configPath)

	entry, ok := cfg.FindAgent("bob")
	if !ok {
		t.Fatal("FindAgent returned false, want true")
	}
	if entry.Name != "bob" {
		t.Errorf("Name = %q, want %q", entry.Name, "bob")
	}

	_, ok = cfg.FindAgent("ghost")
	if ok {
		t.Error("FindAgent returned true for non-existent agent")
	}
}

func TestWriteAgentDisabled(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents", "testbot")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	agentTOML := filepath.Join(agentDir, "agent.toml")
	if err := os.WriteFile(agentTOML, []byte(`[agent]
id = "testbot"
`), 0o600); err != nil {
		t.Fatalf("write agent.toml: %v", err)
	}

	// Write disabled = true.
	if err := config.WriteAgentDisabled(agentDir, true); err != nil {
		t.Fatalf("WriteAgentDisabled(true): %v", err)
	}

	// Verify the written value by re-reading the raw file.
	data, _ := os.ReadFile(agentTOML)
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		t.Fatalf("parse written TOML: %v", err)
	}
	if v, ok := raw["disabled"].(bool); !ok || !v {
		t.Errorf("disabled = %v, want true", raw["disabled"])
	}

	// Write disabled = false.
	if err := config.WriteAgentDisabled(agentDir, false); err != nil {
		t.Fatalf("WriteAgentDisabled(false): %v", err)
	}

	data2, _ := os.ReadFile(agentTOML)
	var raw2 map[string]any
	if _, err := toml.Decode(string(data2), &raw2); err != nil {
		t.Fatalf("parse written TOML: %v", err)
	}
	if v, ok := raw2["disabled"].(bool); !ok || v {
		t.Errorf("disabled = %v, want false", raw2["disabled"])
	}
}

func TestWriteAgentDisabled_MissingAgentDir(t *testing.T) {
	err := config.WriteAgentDisabled("/ghost/agent/dir", true)
	if err == nil {
		t.Error("expected error for missing agent dir, got nil")
	}
}

func TestLoadHubConfig(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configPath := filepath.Join(zlawDir, "zlaw.toml")
	if err := os.WriteFile(configPath, []byte(`[hub]
name = "my-hub"
description = "test hub"

[[agents]]
name = "manager"
dir = "agents/manager"
restart_policy = "always"

[[agents]]
name = "worker"
workspace = "workspaces/worker"

[nats]
listen = "0.0.0.0:4222"
`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		t.Fatalf("LoadHubConfig: %v", err)
	}
	if cfg.Hub.Name != "my-hub" {
		t.Errorf("Hub.Name = %q, want %q", cfg.Hub.Name, "my-hub")
	}
	if cfg.Hub.Description != "test hub" {
		t.Errorf("Hub.Description = %q, want %q", cfg.Hub.Description, "test hub")
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("Agents len = %d, want 2", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "manager" {
		t.Errorf("Agents[0].Name = %q", cfg.Agents[0].Name)
	}
	if cfg.Agents[0].RestartPolicy != config.RestartAlways {
		t.Errorf("Agents[0].RestartPolicy = %v, want RestartAlways", cfg.Agents[0].RestartPolicy)
	}
	if cfg.Agents[1].Workspace != "workspaces/worker" {
		t.Errorf("Agents[1].Workspace = %q", cfg.Agents[1].Workspace)
	}
	if cfg.NATS.Listen != "0.0.0.0:4222" {
		t.Errorf("NATS.Listen = %q", cfg.NATS.Listen)
	}
}

func TestLoadHubConfig_NotFound(t *testing.T) {
	_, err := config.LoadHubConfig("/ghost/zlaw.toml")
	if err == nil {
		t.Error("expected error for missing zlaw.toml, got nil")
	}
}

func TestDefaultHubConfigPath(t *testing.T) {
	dir := t.TempDir()
	zlawDir := filepath.Join(dir, ".zlaw")
	if err := os.MkdirAll(zlawDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawDir)

	got := config.DefaultHubConfigPath()
	want := filepath.Join(zlawDir, "zlaw.toml")
	if got != want {
		t.Errorf("DefaultHubConfigPath() = %q, want %q", got, want)
	}
}
