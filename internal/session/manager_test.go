package session_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/session"
)

// mockRunner is a test AgentRunner that records calls and returns a fixed response.
type mockRunner struct {
	mu     sync.Mutex
	calls  []runRecord
	result string
	delay  time.Duration
	doneCh chan struct{} // closed after each RunStream completes
}

type runRecord struct {
	sessionID string
	input     string
}

func newMockRunner(result string) *mockRunner {
	return &mockRunner{result: result, doneCh: make(chan struct{}, 8)}
}

func (m *mockRunner) RunStream(
	_ context.Context,
	sessionID, input, _ string,
	handler llm.StreamHandler,
) (string, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.mu.Lock()
	m.calls = append(m.calls, runRecord{sessionID: sessionID, input: input})
	m.mu.Unlock()
	if handler != nil {
		handler("delta1 ")
		handler("delta2")
	}
	m.doneCh <- struct{}{}
	return m.result, nil
}

func (m *mockRunner) Calls() []runRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]runRecord, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestManager_GetOrCreate_NewSession(t *testing.T) {
	runner := newMockRunner("hello")
	mgr := session.NewManager(runner, func() string { return "system" }, nil)
	ctx := context.Background()

	s := mgr.GetOrCreate(ctx, "sess1")
	if s == nil {
		t.Fatal("GetOrCreate returned nil")
	}
	if s.ID != "sess1" {
		t.Fatalf("want ID=sess1, got %s", s.ID)
	}
	if s.Broadcaster == nil {
		t.Fatal("Broadcaster is nil")
	}
}

func TestManager_GetOrCreate_Idempotent(t *testing.T) {
	runner := newMockRunner("hello")
	mgr := session.NewManager(runner, func() string { return "system" }, nil)
	ctx := context.Background()

	s1 := mgr.GetOrCreate(ctx, "sess1")
	s2 := mgr.GetOrCreate(ctx, "sess1")
	if s1 != s2 {
		t.Fatal("GetOrCreate returned different sessions for the same ID")
	}
}

func TestManager_Submit_CallsRunner(t *testing.T) {
	runner := newMockRunner("response")
	mgr := session.NewManager(runner, func() string { return "sys" }, nil)
	ctx := context.Background()

	mgr.Submit(ctx, "sess1", "hello world")

	// Wait for the runner to be called.
	select {
	case <-runner.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner to be called")
	}

	calls := runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 runner call, got %d", len(calls))
	}
	if calls[0].sessionID != "sess1" {
		t.Errorf("want sessionID=sess1, got %s", calls[0].sessionID)
	}
	if calls[0].input != "hello world" {
		t.Errorf("want input='hello world', got %q", calls[0].input)
	}
}

func TestManager_Submit_BroadcastsEvents(t *testing.T) {
	runner := newMockRunner("final response")
	mgr := session.NewManager(runner, func() string { return "sys" }, nil)
	ctx := context.Background()

	sink := &mockSink{caps: session.ChannelCaps{Streaming: true}}
	s := mgr.GetOrCreate(ctx, "sess2")
	s.Broadcaster.Add(sink)

	mgr.Submit(ctx, "sess2", "ping")

	// Wait for the turn to complete.
	select {
	case <-runner.doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner")
	}

	// Give broadcaster goroutines a moment to deliver.
	time.Sleep(20 * time.Millisecond)

	events := sink.Events()
	var deltaCount, doneCount int
	for _, e := range events {
		switch e.Type {
		case session.EventAssistantDelta:
			deltaCount++
		case session.EventAssistantDone:
			doneCount++
		}
	}
	if deltaCount < 1 {
		t.Errorf("want ≥1 delta events, got %d", deltaCount)
	}
	if doneCount != 1 {
		t.Errorf("want 1 done event, got %d", doneCount)
	}
}

func TestManager_Submit_DropWhenFull(t *testing.T) {
	// Use a slow runner so the queue fills up.
	runner := newMockRunner("ok")
	runner.delay = 100 * time.Millisecond
	mgr := session.NewManager(runner, func() string { return "sys" }, nil)
	ctx := context.Background()

	// Submit 20 turns; the channel holds 8.
	// The excess should be dropped without panic.
	for i := 0; i < 20; i++ {
		mgr.Submit(ctx, "sess3", "msg")
	}
	// Drain doneCh to avoid goroutine leak in test.
	// Only a subset of messages will have been queued.
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-runner.doneCh:
		case <-timeout:
			return // test passed: no panic
		}
	}
}
