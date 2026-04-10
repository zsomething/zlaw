package session

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ChannelCaps declares the output capabilities of a sink.
type ChannelCaps struct {
	Streaming       bool // can receive incremental EventAssistantDelta events
	TypingIndicator bool // supports a "typing" or progress indicator
}

// OutputSink is implemented by every output channel (CLI attach connection,
// Telegram chat, etc.).
type OutputSink interface {
	// Capabilities returns the static capability set for this sink.
	Capabilities() ChannelCaps
	// SendTyping tells the remote channel to show a typing/progress indicator.
	// Implementations that don't support typing should return nil.
	SendTyping(ctx context.Context) error
	// Send delivers an event to this sink.
	// The broadcaster only sends EventAssistantDelta to streaming sinks.
	Send(ctx context.Context, e Event) error
	// Close releases resources held by the sink.
	Close() error
}

// Broadcaster fans events out to a dynamic set of OutputSinks.
// It is safe for concurrent use.
type Broadcaster struct {
	mu     sync.Mutex
	sinks  []OutputSink
	logger *slog.Logger
}

// NewBroadcaster returns an empty Broadcaster. logger may be nil (slog.Default() is used).
func NewBroadcaster(logger *slog.Logger) *Broadcaster {
	if logger == nil {
		logger = slog.Default()
	}
	return &Broadcaster{logger: logger}
}

// Add registers a sink. Duplicate registrations are ignored.
func (b *Broadcaster) Add(s OutputSink) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, existing := range b.sinks {
		if existing == s {
			return
		}
	}
	b.sinks = append(b.sinks, s)
}

// Remove unregisters a sink. No-op if not registered.
func (b *Broadcaster) Remove(s OutputSink) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.removeLocked(s)
}

func (b *Broadcaster) removeLocked(s OutputSink) {
	for i, existing := range b.sinks {
		if existing == s {
			b.sinks = append(b.sinks[:i], b.sinks[i+1:]...)
			return
		}
	}
}

// Broadcast sends e to all registered sinks.
// EventAssistantDelta is only delivered to streaming sinks.
// Sinks that return an error are closed and removed from the broadcaster.
func (b *Broadcaster) Broadcast(ctx context.Context, e Event) {
	b.mu.Lock()
	sinks := make([]OutputSink, len(b.sinks))
	copy(sinks, b.sinks)
	b.mu.Unlock()

	var failed []OutputSink
	for _, s := range sinks {
		if e.Type == EventAssistantDelta && !s.Capabilities().Streaming {
			continue
		}
		if err := s.Send(ctx, e); err != nil {
			b.logger.Warn("broadcaster: sink send error", "error", err)
			failed = append(failed, s)
		}
	}
	for _, s := range failed {
		_ = s.Close()
		b.Remove(s)
	}
}

// StartTyping launches a background goroutine that calls SendTyping on all
// sinks that declare TypingIndicator=true, at the given interval.
// Returns a CancelFunc; call it to stop the loop.
func (b *Broadcaster) StartTyping(ctx context.Context, interval time.Duration) context.CancelFunc {
	typingCtx, cancel := context.WithCancel(ctx)
	go func() {
		// Send immediately, then on each tick.
		b.sendTypingAll(typingCtx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				b.sendTypingAll(typingCtx)
			}
		}
	}()
	return cancel
}

func (b *Broadcaster) sendTypingAll(ctx context.Context) {
	b.mu.Lock()
	sinks := make([]OutputSink, len(b.sinks))
	copy(sinks, b.sinks)
	b.mu.Unlock()
	for _, s := range sinks {
		if !s.Capabilities().TypingIndicator {
			continue
		}
		if err := s.SendTyping(ctx); err != nil {
			b.logger.Warn("broadcaster: typing indicator error", "error", err)
		}
	}
}
