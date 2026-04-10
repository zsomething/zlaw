// Package skills handles discovery and loading of markdown-based agent skills.
package skills

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a single discovered skill loaded from a SKILL.md file.
type Skill struct {
	Name        string // from frontmatter
	Description string // from frontmatter — answers "when should I activate this?"
	Body        string // full markdown body (excluding frontmatter)
	Path        string // absolute path to the SKILL.md file
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Discover finds all skills available to an agent, applying two-level resolution:
//
//   - $zlawHome/skills/<name>/SKILL.md — shared across all agents
//   - $zlawHome/agents/<agentName>/skills/<name>/SKILL.md — agent-specific
//
// When the same skill name exists in both locations, the agent-local version
// wins and a warning is logged. logger may be nil (warnings are silently dropped).
func Discover(zlawHome, agentName string, logger *slog.Logger) ([]Skill, error) {
	if logger == nil {
		logger = slog.Default()
	}
	global, err := loadDir(filepath.Join(zlawHome, "skills"), logger)
	if err != nil {
		return nil, fmt.Errorf("skills discover global: %w", err)
	}

	local, err := loadDir(filepath.Join(zlawHome, "agents", agentName, "skills"), logger)
	if err != nil {
		return nil, fmt.Errorf("skills discover local: %w", err)
	}

	// Merge: agent-local wins on name conflict.
	merged := make(map[string]Skill, len(global)+len(local))
	for _, s := range global {
		merged[s.Name] = s
	}
	for _, s := range local {
		if existing, ok := merged[s.Name]; ok {
			logger.Warn("skill name conflict: agent-local overrides global",
				"name", s.Name,
				"global_path", existing.Path,
				"local_path", s.Path,
			)
		}
		merged[s.Name] = s
	}

	result := make([]Skill, 0, len(merged))
	for _, s := range merged {
		result = append(result, s)
	}

	// Stable sort by name.
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Name < result[i].Name {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// loadDir reads all <dir>/<name>/SKILL.md files and returns the parsed skills.
// Missing or empty directories are treated as no skills (no error).
func loadDir(dir string, logger *slog.Logger) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills loaddir %s: %w", dir, err)
	}

	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		s, err := Load(path)
		if err != nil {
			logger.Warn("skip invalid skill file", "path", path, "error", err)
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// Load parses a single SKILL.md file. The file must have YAML frontmatter
// with at least a name and description field.
func Load(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("skill load %s: %w", path, err)
	}
	fm, body, err := parseSkillFile(data)
	if err != nil {
		return Skill{}, fmt.Errorf("skill load %s: %w", path, err)
	}
	if fm.Name == "" {
		return Skill{}, fmt.Errorf("skill load %s: missing 'name' in frontmatter", path)
	}
	if fm.Description == "" {
		return Skill{}, fmt.Errorf("skill load %s: missing 'description' in frontmatter", path)
	}
	return Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Body:        body,
		Path:        path,
	}, nil
}

// parseSkillFile splits a SKILL.md into frontmatter and body.
// Frontmatter is the YAML block between the first pair of "---" lines.
func parseSkillFile(data []byte) (skillFrontmatter, string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return skillFrontmatter{}, "", fmt.Errorf("missing opening frontmatter delimiter")
	}

	var fmLines []string
	closed := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			closed = true
			break
		}
		fmLines = append(fmLines, line)
	}
	if !closed {
		return skillFrontmatter{}, "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &fm); err != nil {
		return skillFrontmatter{}, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return skillFrontmatter{}, "", fmt.Errorf("scan: %w", err)
	}

	body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return fm, body, nil
}
