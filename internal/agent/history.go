package agent

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
)

// History stores per-session conversation turns.
// It is safe for concurrent use.
// When a SessionStore is configured, messages are persisted on every Append
// and loaded from the store on first access.
type History struct {
	mu      sync.Mutex
	cache   map[string][]llm.Message
	store   SessionStore // nil = in-memory only
	channel string       // adapter channel name written to session metadata
	loaded  map[string]bool
	logger  *slog.Logger
}

// NewHistory returns an in-memory-only History.
func NewHistory() *History {
	return &History{
		cache:  make(map[string][]llm.Message),
		loaded: make(map[string]bool),
		logger: slog.Default(),
	}
}

// NewHistoryWithStore returns a History backed by store for durable persistence.
// channel identifies the adapter (e.g. "cli", "telegram") and is written to
// session metadata on first use.
func NewHistoryWithStore(store SessionStore, channel string) *History {
	return &History{
		cache:   make(map[string][]llm.Message),
		store:   store,
		channel: channel,
		loaded:  make(map[string]bool),
		logger:  slog.Default(),
	}
}

// Append adds a message to the session's history and persists it if a store is
// configured. Persistence errors are logged but do not block the caller.
func (h *History) Append(sessionID string, msg llm.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cache[sessionID] = append(h.cache[sessionID], msg)

	if h.store != nil {
		if err := h.store.Append(sessionID, msg); err != nil {
			h.logger.Warn("history: persist failed", "session_id", sessionID, "error", err)
		}
		if err := h.store.UpdateMeta(sessionID, func(m *SessionMeta) {
			now := time.Now().UTC()
			if m.CreatedAt.IsZero() {
				m.SessionID = sessionID
				m.Channel = h.channel
				m.CreatedAt = now
				m.Title = extractTitle(msg)
			}
			m.UpdatedAt = now
			m.MessageCount++
		}); err != nil {
			h.logger.Warn("history: meta update failed", "session_id", sessionID, "error", err)
		}
	}
}

// RecordUsage adds token counts from one agent turn to the session metadata.
// No-op when no store is configured.
func (h *History) RecordUsage(sessionID string, usage llm.Usage) {
	if h.store == nil {
		return
	}
	if err := h.store.UpdateMeta(sessionID, func(m *SessionMeta) {
		m.TotalInputTokens += usage.InputTokens
		m.TotalOutputTokens += usage.OutputTokens
	}); err != nil {
		h.logger.Warn("history: usage update failed", "session_id", sessionID, "error", err)
	}
}

// extractTitle returns a short label from the first user message content.
func extractTitle(msg llm.Message) string {
	if msg.Role != llm.RoleUser {
		return ""
	}
	for _, b := range msg.Content {
		if b.Text != "" {
			t := strings.TrimSpace(b.Text)
			if len(t) > 80 {
				t = t[:80] + "…"
			}
			return t
		}
	}
	return ""
}

// Get returns a copy of the message slice for the given session.
// On the first access for a session, messages are loaded from the store if one
// is configured.
func (h *History) Get(sessionID string) []llm.Message {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.store != nil && !h.loaded[sessionID] {
		h.loaded[sessionID] = true
		msgs, err := h.store.Load(sessionID)
		if err != nil {
			h.logger.Warn("history: load failed", "session_id", sessionID, "error", err)
		} else if len(msgs) > 0 {
			// Merge: prefer whatever is already in cache (shouldn't happen on first
			// access, but guard anyway) then append loaded messages.
			h.cache[sessionID] = append(msgs, h.cache[sessionID]...)
		}
	}

	msgs := h.cache[sessionID]
	if len(msgs) == 0 {
		return nil
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}

// Lines returns a human-readable representation of the session's conversation
// history as a slice of strings. Tool-result messages (role=tool) are omitted;
// tool calls inside assistant messages are shown as [tool: name].
// Returns nil when there is no history.
func (h *History) Lines(sessionID string) []string {
	msgs := h.Get(sessionID)
	if len(msgs) == 0 {
		return nil
	}
	var lines []string
	for i, m := range msgs {
		switch m.Role {
		case llm.RoleTool:
			// internal tool-result turn — skip
		case llm.RoleUser:
			if text := m.TextContent(); text != "" {
				lines = append(lines, fmt.Sprintf("[%d] you: %s", i+1, text))
			}
		case llm.RoleAssistant:
			if text := m.TextContent(); text != "" {
				lines = append(lines, fmt.Sprintf("[%d] assistant: %s", i+1, text))
			}
			for _, tu := range m.ToolUses() {
				lines = append(lines, fmt.Sprintf("[%d] assistant: [tool: %s]", i+1, tu.Name))
			}
		}
	}
	return lines
}

// Clear removes all in-memory messages for the given session and archives
// the on-disk session files. After Clear, Get returns nil for this session
// and new Append calls start a fresh log. Archive errors are logged but do
// not block the clear.
func (h *History) Clear(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.cache, sessionID)
	// Mark as loaded so the next Get does not attempt a disk reload before
	// the first new Append (the archived file is gone, but this avoids the
	// stat round-trip).
	h.loaded[sessionID] = true
	if h.store != nil {
		if err := h.store.Archive(sessionID); err != nil {
			h.logger.Warn("history: archive failed", "session_id", sessionID, "error", err)
		}
	}
}

// SessionDir returns the session directory derived from ZLAW_AGENT_HOME.
func SessionDir() (string, error) {
	return filepath.Join(config.AgentHome(), "sessions"), nil
}
