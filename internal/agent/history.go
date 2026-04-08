package agent

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/chickenzord/zlaw/internal/llm"
)

// History stores per-session conversation turns.
// It is safe for concurrent use.
// When a SessionStore is configured, messages are persisted on every Append
// and loaded from the store on first access.
type History struct {
	mu      sync.Mutex
	cache   map[string][]llm.Message
	store   SessionStore // nil = in-memory only
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
func NewHistoryWithStore(store SessionStore) *History {
	return &History{
		cache:  make(map[string][]llm.Message),
		store:  store,
		loaded: make(map[string]bool),
		logger: slog.Default(),
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
	}
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

// Clear removes all in-memory messages for the given session.
// Persisted data on disk is not deleted.
func (h *History) Clear(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.cache, sessionID)
	delete(h.loaded, sessionID)
}

// SessionDir returns the conventional session directory for a named agent.
// Path: ~/.zlaw/agents/<agentName>/sessions
func SessionDir(agentName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("history: resolve home: %w", err)
	}
	return fmt.Sprintf("%s/.zlaw/agents/%s/sessions", home, agentName), nil
}
