package agent

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zsomething/zlaw/internal/config"
)

// Memory is a single long-term memory entry stored per agent.
type Memory struct {
	ID        string    `yaml:"id"`
	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
	Tags      []string  `yaml:"tags,omitempty"`
	Content   string    `yaml:"-"`
}

// MemoryStore is the interface for persisting and querying agent memories.
type MemoryStore interface {
	// Save creates or overwrites the memory with the given ID.
	Save(m Memory) error
	// Delete removes the memory with the given ID.
	// Returns nil if the memory does not exist.
	Delete(id string) error
	// Search returns memories whose content or tags contain any of the keywords.
	// Matching is case-insensitive. Returns all memories when keywords is empty.
	Search(keywords []string) ([]Memory, error)
	// List returns all memories in no guaranteed order.
	List() ([]Memory, error)
}

// MemoryDir returns the memory directory derived from ZLAW_AGENT_HOME.
func MemoryDir() (string, error) {
	return filepath.Join(config.AgentHome(), "memories"), nil
}

// MarkdownFileStore stores each memory as a Markdown file with YAML frontmatter.
// File path: <baseDir>/<id>.md
// Format:
//
//	---
//	id: <id>
//	created_at: <RFC3339>
//	updated_at: <RFC3339>
//	tags:
//	  - tag1
//	  - tag2
//	---
//	<content>
type MarkdownFileStore struct {
	baseDir string
}

// NewMarkdownFileStore returns a store that writes to baseDir.
// The directory is created on first use.
func NewMarkdownFileStore(baseDir string) *MarkdownFileStore {
	return &MarkdownFileStore{baseDir: baseDir}
}

func (s *MarkdownFileStore) filePath(id string) string {
	return filepath.Join(s.baseDir, id+".md")
}

// Save writes the memory to disk as <baseDir>/<id>.md.
func (s *MarkdownFileStore) Save(m Memory) error {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return fmt.Errorf("memory store: mkdir %s: %w", s.baseDir, err)
	}

	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	fm, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("memory store: marshal frontmatter %s: %w", m.ID, err)
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(fm)
	sb.WriteString("---\n")
	sb.WriteString(m.Content)
	if len(m.Content) > 0 && !strings.HasSuffix(m.Content, "\n") {
		sb.WriteByte('\n')
	}

	if err := os.WriteFile(s.filePath(m.ID), []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("memory store: write %s: %w", m.ID, err)
	}
	return nil
}

// Delete removes the memory file. Returns nil when the file does not exist.
func (s *MarkdownFileStore) Delete(id string) error {
	err := os.Remove(s.filePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("memory store: delete %s: %w", id, err)
	}
	return nil
}

// List returns all memories by reading every *.md file in baseDir.
func (s *MarkdownFileStore) List() ([]Memory, error) {
	entries, err := os.ReadDir(s.baseDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memory store: readdir %s: %w", s.baseDir, err)
	}

	var memories []Memory
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		m, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// Search returns memories matching any of the keywords (case-insensitive).
// Matching is done against content and tags. Returns all memories when keywords
// is empty.
func (s *MarkdownFileStore) Search(keywords []string) ([]Memory, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(keywords) == 0 {
		return all, nil
	}

	lower := make([]string, len(keywords))
	for i, k := range keywords {
		lower[i] = strings.ToLower(k)
	}

	var results []Memory
	for _, m := range all {
		if matchesAny(m, lower) {
			results = append(results, m)
		}
	}
	return results, nil
}

func matchesAny(m Memory, lowerKeywords []string) bool {
	contentLow := strings.ToLower(m.Content)
	for _, kw := range lowerKeywords {
		if strings.Contains(contentLow, kw) {
			return true
		}
		for _, tag := range m.Tags {
			if strings.Contains(strings.ToLower(tag), kw) {
				return true
			}
		}
	}
	return false
}

// Load parses and returns the memory with the given ID.
// Returns an error if the file does not exist.
func (s *MarkdownFileStore) Load(id string) (Memory, error) {
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		return Memory{}, fmt.Errorf("memory store: read %s: %w", id, err)
	}
	return parseMemoryFile(data)
}

// parseMemoryFile parses a memory file that starts with YAML frontmatter
// delimited by "---" lines. Everything after the closing "---" is the content.
func parseMemoryFile(data []byte) (Memory, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	// Expect opening "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return Memory{}, fmt.Errorf("memory store: missing frontmatter delimiter")
	}

	var fmLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		fmLines = append(fmLines, line)
	}

	var m Memory
	if err := yaml.Unmarshal([]byte(strings.Join(fmLines, "\n")), &m); err != nil {
		return Memory{}, fmt.Errorf("memory store: parse frontmatter: %w", err)
	}

	var contentLines []string
	for scanner.Scan() {
		contentLines = append(contentLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return Memory{}, fmt.Errorf("memory store: scan: %w", err)
	}

	m.Content = strings.Join(contentLines, "\n")
	// Trim trailing newline added by Save
	m.Content = strings.TrimRight(m.Content, "\n")

	return m, nil
}
