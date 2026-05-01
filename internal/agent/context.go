package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/skills"
)

// BuildSelfIdentitySticky returns a StickyBlock that injects the agent's own
// identity (id + roles) into every system prompt. This gives the agent
// self-awareness without relying on personality files.
func BuildSelfIdentitySticky(id string, roles []string) StickyBlock {
	content := strings.ReplaceAll(StickySelfIdentity, "{agent_id}", id)
	var rolesStr string
	if len(roles) > 0 {
		rolesStr = strings.Join(roles, ", ")
	} else {
		rolesStr = "(none)"
	}
	content = strings.ReplaceAll(content, "{agent_roles}", rolesStr)
	return StickyBlock{
		Name:    "self-identity",
		Content: content,
	}
}

// StickyBlock is a named, framework-level instruction block injected at the
// head of every system prompt. Content lives in Go source, not markdown files,
// so user personality files cannot override it.
type StickyBlock struct {
	Name    string
	Content string
}

// BuildPrefill expands a list of context sources into a preamble string that
// can be prepended to the first user message of a new session. Returns an
// empty string when sources is empty.
//
// Supported sources:
//   - "cwd"              — current working directory
//   - "datetime"         — current date and time (RFC3339)
//   - "file:<path>"      — contents of a file relative to agentDir
func BuildPrefill(agentDir string, sources []string) (string, error) {
	if len(sources) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("[Session context]\n")
	for _, src := range sources {
		switch {
		case src == "cwd":
			cwd, err := os.Getwd()
			if err != nil {
				return "", fmt.Errorf("prefill cwd: %w", err)
			}
			fmt.Fprintf(&b, "cwd: %s\n", cwd)
		case src == "datetime":
			fmt.Fprintf(&b, "datetime: %s\n", time.Now().Format(time.RFC3339))
		case strings.HasPrefix(src, "file:"):
			rel := strings.TrimPrefix(src, "file:")
			path := filepath.Join(agentDir, rel)
			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("prefill file %q: %w", path, err)
			}
			fmt.Fprintf(&b, "file:%s\n%s\n", rel, strings.TrimSpace(string(data)))
		default:
			return "", fmt.Errorf("prefill: unknown source %q (supported: cwd, datetime, file:<path>)", src)
		}
	}
	return b.String(), nil
}

// StickyProactiveMemorySave is the hardcoded instruction block injected as
// cache checkpoint 1 when memory.proactive_save is enabled in agent.toml.
// It instructs the agent to call memory_save when it learns something worth
// retaining. Content is intentionally in Go source (not a user-editable file)
// so it cannot be silently broken by a personality edit.
const StickyProactiveMemorySave = `[Memory behavior]
When you learn something worth retaining — user preferences, facts, decisions, recurring context — call memory_save immediately. Keep memories short, factual, and tagged. Do not save transient info or things already in session history.`

// StickySelfIdentity is the hardcoded instruction block injected as cache
// checkpoint 1 that tells the agent its own identity (ID + roles).
const StickySelfIdentity = `[Self identity]
You are agent {agent_id}. Roles: {agent_roles}.
When delegating tasks, use the agent_list tool to find other agents — agent_list marks "is_self: true" on the current agent.
Never impersonate another agent.]`

// BuildSkillsSection returns a formatted [Available Skills] block for injection
// into the system prompt. Each skill is listed as:
//
//   - <name>: <description>
//
// Returns an empty string when skills is empty.
func BuildSkillsSection(discovered []skills.Skill) string {
	if len(discovered) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Available Skills]\n")
	for _, s := range discovered {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	return b.String()
}

// BuildMemoriesSection returns a formatted [Memories] block for injection into
// the system prompt. Memories are listed in reverse-update order (most recent
// first). Each entry is formatted as:
//
//   - <content> #tag1 #tag2
//
// When maxTokens > 0 entries are truncated so the block fits within the budget
// (1 token ≈ 4 chars). Returns an empty string when store is nil or has no
// memories.
func BuildMemoriesSection(store MemoryStore, maxTokens int) (string, error) {
	if store == nil {
		return "", nil
	}
	memories, err := store.List()
	if err != nil {
		return "", fmt.Errorf("memories section: %w", err)
	}
	if len(memories) == 0 {
		return "", nil
	}

	// Sort: most recently updated first.
	for i := 0; i < len(memories)-1; i++ {
		for j := i + 1; j < len(memories); j++ {
			if memories[j].UpdatedAt.After(memories[i].UpdatedAt) {
				memories[i], memories[j] = memories[j], memories[i]
			}
		}
	}

	const header = "[Memories]\n"
	const charsPerToken = 4
	budgetChars := 0
	if maxTokens > 0 {
		budgetChars = maxTokens * charsPerToken
	}

	var b strings.Builder
	b.WriteString(header)
	used := len(header)

	for _, m := range memories {
		line := formatMemoryLine(m)
		if budgetChars > 0 && used+len(line) > budgetChars {
			break
		}
		b.WriteString(line)
		used += len(line)
	}

	if b.Len() == len(header) {
		return "", nil // no entries fit
	}
	return b.String(), nil
}

func formatMemoryLine(m Memory) string {
	var sb strings.Builder
	sb.WriteString("- ")
	// Collapse multi-line content to a single line.
	sb.WriteString(strings.ReplaceAll(strings.TrimSpace(m.Content), "\n", " "))
	for _, tag := range m.Tags {
		sb.WriteString(" #")
		sb.WriteString(tag)
	}
	sb.WriteByte('\n')
	return sb.String()
}

// BuildSystemPrompt assembles the full system prompt from sticky blocks and
// personality files. Sticky blocks are prepended before SOUL.md and
// IDENTITY.md. Pass nil sticky for the personality-only string.
//
// When agentID is non-empty, a [System Identity] block is appended to the
// identity section so the agent always knows its own identifier.
func BuildSystemPrompt(sticky []StickyBlock, p config.Personality, agentID string) string {
	var b strings.Builder
	for _, s := range sticky {
		if c := strings.TrimSpace(s.Content); c != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(c)
		}
	}
	if p.Soul != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(p.Soul))
	}
	if p.Identity != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(p.Identity))
	}
	// Append system identity block with agent ID if provided.
	if agentID != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[System Identity]\nagent_id: %s", agentID)
	}
	return b.String()
}

// BuildSystemSections returns the system prompt as structured sections with
// cache checkpoint markers for backends that support multi-block system
// prompts (e.g. Anthropic prompt caching).
//
// Section layout (stable → volatile):
//   - Section 1 (CacheCheckpoint=true): sticky blocks — never changes (cache checkpoint 1)
//   - Section 2 (CacheCheckpoint=true): SOUL.md + IDENTITY.md — rarely changes (cache checkpoint 2)
//
// Empty sections are omitted. Returns nil if both inputs are empty.
func BuildSystemSections(sticky []StickyBlock, p config.Personality) []llm.SystemSection {
	var sections []llm.SystemSection

	// Section 1: sticky blocks
	var sb strings.Builder
	for _, s := range sticky {
		if c := strings.TrimSpace(s.Content); c != "" {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c)
		}
	}
	if sb.Len() > 0 {
		sections = append(sections, llm.SystemSection{
			Content:         sb.String(),
			CacheCheckpoint: true,
		})
	}

	// Section 2: personality
	var pb strings.Builder
	if p.Soul != "" {
		pb.WriteString(strings.TrimSpace(p.Soul))
	}
	if p.Identity != "" {
		if pb.Len() > 0 {
			pb.WriteString("\n\n")
		}
		pb.WriteString(strings.TrimSpace(p.Identity))
	}
	if pb.Len() > 0 {
		sections = append(sections, llm.SystemSection{
			Content:         pb.String(),
			CacheCheckpoint: true,
		})
	}

	return sections
}
