package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/messaging"
)

// stubRunner implements agent.HubTaskRunner for tests.
type stubRunner struct {
	output string
	err    error
	calls  int
}

func (s *stubRunner) Run(_ context.Context, sessionID, input, systemPrompt string) (string, error) {
	s.calls++
	return s.output, s.err
}

var _ agent.HubTaskRunner = (*stubRunner)(nil)

// closureHubTaskRunner adapts a closure to HubTaskRunner.
type closureHubTaskRunner struct {
	fn func(ctx context.Context, sessionID, input, systemPrompt string) (string, error)
}

func (r *closureHubTaskRunner) Run(ctx context.Context, sessionID, input, systemPrompt string) (string, error) {
	return r.fn(ctx, sessionID, input, systemPrompt)
}

var _ agent.HubTaskRunner = (*closureHubTaskRunner)(nil)

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestHubClient_RoundTrip verifies that HubClient receives a task envelope,
// calls the runner, and publishes the reply back to the reply-to subject.
func TestHubClient_RoundTrip(t *testing.T) {
	cm := messaging.NewChanMessenger()
	calls := make(chan struct{ Input, SessionID, SystemPrompt string }, 1)
	runner := &closureHubTaskRunner{
		fn: func(_ context.Context, sessionID, input, systemPrompt string) (string, error) {
			calls <- struct{ Input, SessionID, SystemPrompt string }{input, sessionID, systemPrompt}
			return "echo:" + input, nil
		},
	}

	logger := slog.New(slog.DiscardHandler)
	client := agent.NewHubClient(
		"worker", "test",
		[]string{"task"},
		nil,
		cm,
		runner,
		func() string { return "system-prompt" },
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- client.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Subscribe to reply subject.
	replyCh := make(chan messaging.TaskReply, 1)
	_, _ = cm.Subscribe(ctx, "_INBOX.test.reply", func(data []byte) {
		var reply messaging.TaskReply
		if err := json.Unmarshal(data, &reply); err != nil {
			t.Logf("malformed reply: %v", err)
			return
		}
		replyCh <- reply
	})
	time.Sleep(50 * time.Millisecond)

	// Publish task envelope.
	env := messaging.TaskEnvelope{
		From: "manager", To: "worker", Task: "say hello",
		SessionID: "session-123", ReplyTo: "_INBOX.test.reply",
	}
	_ = cm.Publish(ctx, "agent.worker.inbox", mustMarshal(env)) //nolint:errcheck

	select {
	case reply := <-replyCh:
		if reply.Output != "echo:say hello" {
			t.Errorf("output = %q, want %q", reply.Output, "echo:say hello")
		}
		if reply.SessionID != "session-123" {
			t.Errorf("session_id = %q, want %q", reply.SessionID, "session-123")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for reply")
	}

	select {
	case call := <-calls:
		if call.Input != "say hello" {
			t.Errorf("runner input = %q, want %q", call.Input, "say hello")
		}
		if call.SystemPrompt != "system-prompt" {
			t.Errorf("system prompt = %q, want %q", call.SystemPrompt, "system-prompt")
		}
	case <-time.After(1 * time.Second):
		t.Error("runner was not called")
	}
}

// TestHubClient_ConcurrentMessages verifies that HubClient handles multiple
// messages arriving quickly without mixing up replies.
func TestHubClient_ConcurrentMessages(t *testing.T) {
	cm := messaging.NewChanMessenger()
	runnerCalls := make(chan string, 3)

	runner := &closureHubTaskRunner{
		fn: func(_ context.Context, _, input, _ string) (string, error) {
			runnerCalls <- input
			return "done:" + input, nil
		},
	}

	logger := slog.New(slog.DiscardHandler)
	client := agent.NewHubClient(
		"worker", "test", nil, nil, cm, runner,
		func() string { return "" }, logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { client.Start(ctx) }() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	// Set up reply collectors for three tasks.
	type taskResult struct {
		idx    int
		output string
	}
	results := make(chan taskResult, 3)
	replySubjects := []string{"_INBOX.reply.0", "_INBOX.reply.1", "_INBOX.reply.2"}
	for i, subj := range replySubjects {
		idx := i
		s := subj
		_, _ = cm.Subscribe(ctx, s, func(data []byte) {
			var reply messaging.TaskReply
			if err := json.Unmarshal(data, &reply); err != nil {
				return
			}
			results <- taskResult{idx: idx, output: reply.Output}
		})
	}
	time.Sleep(50 * time.Millisecond)

	// Publish three envelopes concurrently.
	tasks := []string{"task-a", "task-b", "task-c"}
	for i, task := range tasks {
		env := messaging.TaskEnvelope{
			From: "manager", To: "worker", Task: task,
			SessionID: "s" + string(rune('0'+i)),
			ReplyTo:   replySubjects[i],
		}
		_ = cm.Publish(ctx, "agent.worker.inbox", mustMarshal(env)) //nolint:errcheck
	}

	// Collect replies.
	timeout := time.After(3 * time.Second)
	for received := 0; received < 3; received++ {
		select {
		case r := <-results:
			if r.output == "" {
				t.Errorf("reply %d: empty output", r.idx)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for all replies (got %d/3)", received)
		}
	}

	// Verify runner called three times.
	close(runnerCalls)
	var gotCalls []string
	for input := range runnerCalls {
		gotCalls = append(gotCalls, input)
	}
	if len(gotCalls) != 3 {
		t.Errorf("runner calls = %d, want 3", len(gotCalls))
	}
}

// TestHubClient_RunnerError propagates a runner error into the reply.
func TestHubClient_RunnerError(t *testing.T) {
	cm := messaging.NewChanMessenger()
	wantErr := errors.New("runner failed")
	runner := &stubRunner{err: wantErr}

	logger := slog.New(slog.DiscardHandler)
	client := agent.NewHubClient(
		"worker", "test", nil, nil, cm,
		runner,
		func() string { return "" },
		logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { client.Start(ctx) }() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	replyCh := make(chan messaging.TaskReply, 1)
	_, _ = cm.Subscribe(ctx, "_INBOX.test.err", func(data []byte) {
		var reply messaging.TaskReply
		json.Unmarshal(data, &reply) //nolint:errcheck
		replyCh <- reply
	})
	time.Sleep(50 * time.Millisecond)

	env := messaging.TaskEnvelope{
		From: "manager", To: "worker", Task: "fail",
		SessionID: "s1", ReplyTo: "_INBOX.test.err",
	}
	_ = cm.Publish(ctx, "agent.worker.inbox", mustMarshal(env)) //nolint:errcheck

	select {
	case reply := <-replyCh:
		if reply.Error == "" {
			t.Error("expected error in reply, got none")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for reply")
	}
}

// TestHubClient_MissingReplyTo logs a warning but does not panic.
func TestHubClient_MissingReplyTo(t *testing.T) {
	cm := messaging.NewChanMessenger()
	calls := make(chan struct{}, 1)
	runner := &closureHubTaskRunner{
		fn: func(_ context.Context, _, _, _ string) (string, error) {
			calls <- struct{}{}
			return "ok", nil
		},
	}

	logger := slog.New(slog.DiscardHandler)
	client := agent.NewHubClient(
		"worker", "test", nil, nil, cm, runner,
		func() string { return "" }, logger,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { client.Start(ctx) }() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	// Envelope without ReplyTo — should be silently dropped (no panic).
	env := messaging.TaskEnvelope{
		From: "manager", To: "worker", Task: "do it",
		SessionID: "s1",
		// ReplyTo intentionally missing.
	}
	_ = cm.Publish(ctx, "agent.worker.inbox", mustMarshal(env)) //nolint:errcheck
	time.Sleep(200 * time.Millisecond)

	// Runner should not have been called.
	select {
	case <-calls:
		t.Error("runner should not be called when ReplyTo is missing")
	default:
		// Expected.
	}
}
