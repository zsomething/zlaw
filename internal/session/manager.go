package session

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
)

// AgentRunner is the narrow interface the Manager uses to run agent turns.
// It returns the response text directly to decouple the session package from
// the agent package and avoid import cycles.
type AgentRunner interface {
	RunStream(ctx context.Context, sessionID, input, systemPrompt string,
		handler llm.StreamHandler) (string, error)
}

// turnInput is a single user message queued for processing.
type turnInput struct {
	ctx    context.Context
	input  string
	origin string // e.g. "telegram", "cli-attach"
}

// Session is a live conversation with its own broadcaster and input queue.
type Session struct {
	ID          string
	Broadcaster *Broadcaster
	inputCh     chan turnInput // buffered, size 8
	cancel      context.CancelFunc
}

// Manager creates and drives sessions. Each session runs its own event loop
// in a goroutine, processing turns sequentially.
type Manager struct {
	mu          sync.Mutex
	sessions    map[string]*Session
	runner      AgentRunner
	sysPromptFn func() string
	logger      *slog.Logger
	activeTurns sync.WaitGroup // counts in-flight agent turns
}

// NewManager creates a Manager. logger may be nil (slog.Default() is used).
func NewManager(runner AgentRunner, sysPromptFn func() string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		sessions:    make(map[string]*Session),
		runner:      runner,
		sysPromptFn: sysPromptFn,
		logger:      logger,
	}
}

// GetOrCreate returns the existing session for sessionID, or creates a new one.
// New sessions start their event loop in a background goroutine.
func (m *Manager) GetOrCreate(ctx context.Context, sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	sessCtx, cancel := context.WithCancel(ctx)
	s := &Session{
		ID:          sessionID,
		Broadcaster: NewBroadcaster(m.logger),
		inputCh:     make(chan turnInput, 8),
		cancel:      cancel,
	}
	m.sessions[sessionID] = s
	go m.runSession(sessCtx, s)
	return s
}

// Submit enqueues an input turn for the session identified by sessionID.
// origin identifies the channel that submitted the turn (e.g. "telegram", "cli-attach")
// and is carried through to EventAssistantDone so sinks can adapt their presentation.
// Non-blocking: drops the turn with a warning if the channel buffer is full.
// Returns the session so the caller can add sinks before the turn is processed.
func (m *Manager) Submit(ctx context.Context, sessionID, input, origin string) *Session {
	s := m.GetOrCreate(ctx, sessionID)
	select {
	case s.inputCh <- turnInput{ctx: ctx, input: input, origin: origin}:
	default:
		m.logger.Warn("session: input queue full, dropping turn", "session_id", sessionID)
	}
	return s
}

// runSession is the event loop for a single session. It processes turns
// sequentially and exits when ctx is cancelled.
func (m *Manager) runSession(ctx context.Context, s *Session) {
	log := m.logger.With("session_id", s.ID)
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-s.inputCh:
			if !ok {
				return
			}
			m.processTurn(ctx, s, t, log)
		}
	}
}

func (m *Manager) processTurn(ctx context.Context, s *Session, t turnInput, log *slog.Logger) {
	m.activeTurns.Add(1)
	defer m.activeTurns.Done()
	stopTyping := s.Broadcaster.StartTyping(t.ctx, 4*time.Second)
	defer stopTyping()

	text, err := m.runner.RunStream(t.ctx, s.ID, t.input, m.sysPromptFn(), func(delta string) {
		s.Broadcaster.Broadcast(t.ctx, Event{
			Type:      EventAssistantDelta,
			SessionID: s.ID,
			Data:      delta,
		})
	})

	if err != nil {
		log.Error("session: agent turn failed", "error", err)
		s.Broadcaster.Broadcast(ctx, Event{
			Type:      EventError,
			SessionID: s.ID,
			Data:      err.Error(),
		})
		return
	}

	s.Broadcaster.Broadcast(ctx, Event{
		Type:      EventAssistantDone,
		SessionID: s.ID,
		Data:      text,
		Origin:    t.origin,
		Input:     t.input,
	})
}

// BroadcastAll sends e to every active session's broadcaster.
func (m *Manager) BroadcastAll(ctx context.Context, e Event) {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		s.Broadcaster.Broadcast(ctx, e)
	}
}

// Drain blocks until all in-flight agent turns have finished or ctx is
// cancelled (e.g. the drain timeout fires).
func (m *Manager) Drain(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		m.activeTurns.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
