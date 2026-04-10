package session_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chickenzord/zlaw/internal/session"
)

// mockSink is a test OutputSink that records all interactions.
type mockSink struct {
	mu          sync.Mutex
	caps        session.ChannelCaps
	events      []session.Event
	typingCalls int
	sendErr     error
}

func (m *mockSink) Capabilities() session.ChannelCaps { return m.caps }
func (m *mockSink) SendTyping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingCalls++
	return nil
}
func (m *mockSink) Send(_ context.Context, e session.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.events = append(m.events, e)
	return nil
}
func (m *mockSink) Close() error { return nil }

func (m *mockSink) Events() []session.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]session.Event, len(m.events))
	copy(out, m.events)
	return out
}

func (m *mockSink) TypingCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.typingCalls
}

func newLogger() interface{ Warn(string, ...any); Error(string, ...any) } {
	return nil
}

func TestBroadcaster_AddRemoveBroadcast(t *testing.T) {
	b := session.NewBroadcaster(nil)
	ctx := context.Background()

	s1 := &mockSink{caps: session.ChannelCaps{Streaming: true}}
	s2 := &mockSink{caps: session.ChannelCaps{Streaming: true}}

	b.Add(s1)
	b.Add(s2)

	e := session.Event{Type: session.EventAssistantDone, Data: "hello"}
	b.Broadcast(ctx, e)

	if len(s1.Events()) != 1 {
		t.Fatalf("s1: want 1 event, got %d", len(s1.Events()))
	}
	if len(s2.Events()) != 1 {
		t.Fatalf("s2: want 1 event, got %d", len(s2.Events()))
	}

	b.Remove(s1)
	b.Broadcast(ctx, e)

	if len(s1.Events()) != 1 {
		t.Fatalf("s1 after remove: want 1 event, got %d", len(s1.Events()))
	}
	if len(s2.Events()) != 2 {
		t.Fatalf("s2 after remove s1: want 2 events, got %d", len(s2.Events()))
	}
}

func TestBroadcaster_DuplicateAddIgnored(t *testing.T) {
	b := session.NewBroadcaster(nil)
	ctx := context.Background()

	s := &mockSink{caps: session.ChannelCaps{Streaming: true}}
	b.Add(s)
	b.Add(s) // duplicate
	b.Broadcast(ctx, session.Event{Type: session.EventAssistantDone, Data: "x"})

	if len(s.Events()) != 1 {
		t.Fatalf("want 1 event (not 2 from duplicate), got %d", len(s.Events()))
	}
}

func TestBroadcaster_DeltaOnlyToStreamingSinks(t *testing.T) {
	b := session.NewBroadcaster(nil)
	ctx := context.Background()

	streaming := &mockSink{caps: session.ChannelCaps{Streaming: true}}
	nonStreaming := &mockSink{caps: session.ChannelCaps{Streaming: false}}
	b.Add(streaming)
	b.Add(nonStreaming)

	b.Broadcast(ctx, session.Event{Type: session.EventAssistantDelta, Data: "tok"})

	if len(streaming.Events()) != 1 {
		t.Fatalf("streaming sink: want 1 delta, got %d", len(streaming.Events()))
	}
	if len(nonStreaming.Events()) != 0 {
		t.Fatalf("non-streaming sink: want 0 deltas, got %d", len(nonStreaming.Events()))
	}

	// Non-streaming sink does receive EventAssistantDone.
	b.Broadcast(ctx, session.Event{Type: session.EventAssistantDone, Data: "full"})
	if len(nonStreaming.Events()) != 1 {
		t.Fatalf("non-streaming sink: want 1 done event, got %d", len(nonStreaming.Events()))
	}
}

func TestBroadcaster_FailingSinkRemoved(t *testing.T) {
	b := session.NewBroadcaster(nil)
	ctx := context.Background()

	failing := &mockSink{
		caps:    session.ChannelCaps{Streaming: true},
		sendErr: errors.New("connection reset"),
	}
	good := &mockSink{caps: session.ChannelCaps{Streaming: true}}
	b.Add(failing)
	b.Add(good)

	b.Broadcast(ctx, session.Event{Type: session.EventAssistantDone, Data: "hi"})
	// failing sink errors → removed; good sink receives the event.
	b.Broadcast(ctx, session.Event{Type: session.EventAssistantDone, Data: "hi2"})

	if len(good.Events()) != 2 {
		t.Fatalf("good sink: want 2 events, got %d", len(good.Events()))
	}
	// failing sink never recorded any event (sendErr fires before recording).
	if len(failing.Events()) != 0 {
		t.Fatalf("failing sink: want 0 recorded events, got %d", len(failing.Events()))
	}
}

func TestBroadcaster_StartTyping(t *testing.T) {
	b := session.NewBroadcaster(nil)
	ctx := context.Background()

	s := &mockSink{caps: session.ChannelCaps{TypingIndicator: true}}
	noTyping := &mockSink{caps: session.ChannelCaps{TypingIndicator: false}}
	b.Add(s)
	b.Add(noTyping)

	cancel := b.StartTyping(ctx, 20*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond) // let the goroutine exit

	if s.TypingCalls() < 1 {
		t.Fatalf("typing sink: want ≥1 typing call, got %d", s.TypingCalls())
	}
	if noTyping.TypingCalls() != 0 {
		t.Fatalf("no-typing sink: want 0 typing calls, got %d", noTyping.TypingCalls())
	}
}
