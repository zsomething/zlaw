package messaging

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ChanMessenger is an in-memory Messenger backed by channels.
// It is a test double — never used in production binaries.
type ChanMessenger struct {
	mu   sync.RWMutex
	subs map[string][]*chanSubscription
}

// NewChanMessenger returns a ready-to-use ChanMessenger.
func NewChanMessenger() *ChanMessenger {
	return &ChanMessenger{
		subs: make(map[string][]*chanSubscription),
	}
}

func (m *ChanMessenger) Publish(_ context.Context, subject string, payload []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sub := range m.subs[subject] {
		if !sub.closed {
			data := make([]byte, len(payload))
			copy(data, payload)
			sub.handler(data)
		}
	}
	return nil
}

func (m *ChanMessenger) Subscribe(_ context.Context, subject string, handler func([]byte)) (Subscription, error) {
	sub := &chanSubscription{
		subject: subject,
		handler: handler,
		m:       m,
	}
	m.mu.Lock()
	m.subs[subject] = append(m.subs[subject], sub)
	m.mu.Unlock()
	return sub, nil
}

func (m *ChanMessenger) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	reply := make(chan []byte, 1)

	// Subscribe to a transient reply subject before publishing.
	replySubject := fmt.Sprintf("_INBOX.%p", reply)
	sub, err := m.Subscribe(ctx, replySubject, func(data []byte) {
		select {
		case reply <- data:
		default:
		}
	})
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe() //nolint:errcheck

	// Deliver to subscribers on the target subject, passing replySubject via
	// a simple convention: ChanMessenger does not model NATS reply-to at the
	// wire level. For test purposes, the handler is expected to call
	// Publish(replySubject, ...) itself if it wants to reply.
	// We publish the payload and wait for a reply.
	if err := m.Publish(ctx, subject, payload); err != nil {
		return nil, err
	}

	select {
	case data := <-reply:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *ChanMessenger) removeSub(sub *chanSubscription) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := m.subs[sub.subject]
	for i, s := range subs {
		if s == sub {
			m.subs[sub.subject] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// chanSubscription implements Subscription for ChanMessenger.
type chanSubscription struct {
	subject string
	handler func([]byte)
	m       *ChanMessenger
	mu      sync.Mutex
	closed  bool
}

func (s *chanSubscription) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.m.removeSub(s)
	return nil
}

// JetStream is not implemented for ChanMessenger (returns nil).
// In real usage, JetStream will always be nil for ChanMessenger since it is a
// test double — never used in production binaries.
func (m *ChanMessenger) JetStream() JetStreamer {
	return nil
}

// compile-time interface check
var _ Messenger = (*ChanMessenger)(nil)
