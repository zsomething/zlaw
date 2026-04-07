package agent

import (
	"sync"

	"github.com/chickenzord/zlaw/internal/llm"
)

// History stores per-session conversation turns.
// It is safe for concurrent use.
type History struct {
	mu       sync.Mutex
	sessions map[string][]llm.Message
}

// NewHistory returns an empty History.
func NewHistory() *History {
	return &History{sessions: make(map[string][]llm.Message)}
}

// Append adds a message to the session's history.
func (h *History) Append(sessionID string, msg llm.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[sessionID] = append(h.sessions[sessionID], msg)
}

// Get returns a copy of the message slice for the given session.
func (h *History) Get(sessionID string) []llm.Message {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := h.sessions[sessionID]
	if len(msgs) == 0 {
		return nil
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	return out
}

// Clear removes all messages for the given session.
func (h *History) Clear(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, sessionID)
}
