package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

const weatherSkill = `---
name: weather
description: Handling weather requests. Activate when user asks about weather.
---

When handling weather requests, prefer wttr.in.
`

const timeSkill = `---
name: time
description: Handling time/timezone questions.
---

Use IANA timezone names.
`

const overrideSkill = `---
name: weather
description: Agent-local weather override.
---

Use the local weather API instead.
`

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(weatherSkill), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if s.Name != "weather" {
		t.Errorf("Name = %q, want %q", s.Name, "weather")
	}
	if s.Description != "Handling weather requests. Activate when user asks about weather." {
		t.Errorf("Description = %q", s.Description)
	}
	if s.Body == "" {
		t.Error("Body is empty")
	}
	if s.Path != path {
		t.Errorf("Path = %q, want %q", s.Path, path)
	}
}

func TestLoad_MissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("no frontmatter here\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing frontmatter, got nil")
	}
}

func TestLoad_MissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := "---\ndescription: A skill with no name.\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestDiscover_GlobalOnly(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, "skills"), "weather", weatherSkill)
	writeSkill(t, filepath.Join(home, "skills"), "time", timeSkill)

	skills, err := Discover(home, "myagent", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	// Sorted by name: time < weather
	if skills[0].Name != "time" {
		t.Errorf("skills[0].Name = %q, want %q", skills[0].Name, "time")
	}
	if skills[1].Name != "weather" {
		t.Errorf("skills[1].Name = %q, want %q", skills[1].Name, "weather")
	}
}

func TestDiscover_LocalOverridesGlobal(t *testing.T) {
	home := t.TempDir()
	writeSkill(t, filepath.Join(home, "skills"), "weather", weatherSkill)
	writeSkill(t, filepath.Join(home, "agents", "myagent", "skills"), "weather", overrideSkill)

	skills, err := Discover(home, "myagent", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "Agent-local weather override." {
		t.Errorf("Description = %q, expected local override", skills[0].Description)
	}
}

func TestDiscover_EmptyDirs(t *testing.T) {
	home := t.TempDir()
	skills, err := Discover(home, "myagent", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestDiscover_InvalidSkillSkipped(t *testing.T) {
	home := t.TempDir()
	// Valid skill
	writeSkill(t, filepath.Join(home, "skills"), "time", timeSkill)
	// Invalid skill (no frontmatter)
	writeSkill(t, filepath.Join(home, "skills"), "broken", "no frontmatter\n")

	skills, err := Discover(home, "myagent", nil)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 valid skill, got %d", len(skills))
	}
	if skills[0].Name != "time" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "time")
	}
}
