package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/messaging"
)

// stubRunner is a HubTaskRunner that records the last call and returns a fixed reply.
type stubRunner struct {
	sessionID string
	input     string
	output    string
	err       error
}

func (s *stubRunner) Run(_ context.Context, sessionID, input, _ string) (string, error) {
	s.sessionID = sessionID
	s.input = input
	return s.output, s.err
}

// chanMessenger wraps messaging.ChanMessenger but exposes Publish/Subscribe helpers
// via the Messenger interface for the tests.
type chanMessengerAdapter struct {
	m *messaging.ChanMessenger
}

func (a *chanMessengerAdapter) Publish(ctx context.Context, subject string, payload []byte) error {
	return a.m.Publish(ctx, subject, payload)
}

func (a *chanMessengerAdapter) Subscribe(ctx context.Context, subject string, handler func([]byte)) (messaging.Subscription, error) {
	return a.m.Subscribe(ctx, subject, handler)
}

func (a *chanMessengerAdapter) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	return a.m.Request(ctx, subject, payload, timeout)
}

func (a *chanMessengerAdapter) JetStream() messaging.JetStreamer {
	return nil
}

var _ messaging.Messenger = (*chanMessengerAdapter)(nil)

func newTestMessenger() *chanMessengerAdapter {
	return &chanMessengerAdapter{m: messaging.NewChanMessenger()}
}

func TestHubClient_PublishesRegistration(t *testing.T) {
	m := newTestMessenger()
	runner := &stubRunner{output: "ok"}

	regCh := make(chan []byte, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture the registration before starting the client.
	if _, err := m.Subscribe(ctx, "zlaw.registry", func(data []byte) {
		select {
		case regCh <- data:
		default:
		}
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	go func() {
		hc := agent.NewHubClient("myagent", "1.0", []string{"bash", "glob"}, nil, m, runner, nil, nil)
		_ = hc.Start(ctx)
	}()

	select {
	case data := <-regCh:
		var reg map[string]any
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("unmarshal registration: %v", err)
		}
		if reg["name"] != "myagent" {
			t.Errorf("name = %v, want myagent", reg["name"])
		}
		caps, _ := reg["capabilities"].([]any)
		if len(caps) != 2 {
			t.Errorf("capabilities = %v, want 2 items", caps)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for registration message")
	}
}

func TestHubClient_HandlesInboxTask(t *testing.T) {
	m := newTestMessenger()
	runner := &stubRunner{output: "agent response"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture reply.
	replyCh := make(chan []byte, 1)
	if _, err := m.Subscribe(ctx, "reply.session123", func(data []byte) {
		select {
		case replyCh <- data:
		default:
		}
	}); err != nil {
		t.Fatalf("subscribe reply: %v", err)
	}

	go func() {
		hc := agent.NewHubClient("worker", "1.0", nil, nil, m, runner, nil, nil)
		_ = hc.Start(ctx)
	}()

	// Wait for hub client to subscribe to inbox.
	time.Sleep(50 * time.Millisecond)

	env := messaging.TaskEnvelope{
		SessionID: "session123",
		Task:      "hello",
		ReplyTo:   "reply.session123",
	}
	data, _ := json.Marshal(env)
	if err := m.Publish(ctx, "agent.worker.inbox", data); err != nil {
		t.Fatalf("publish task: %v", err)
	}

	select {
	case replyData := <-replyCh:
		var reply messaging.TaskReply
		if err := json.Unmarshal(replyData, &reply); err != nil {
			t.Fatalf("unmarshal reply: %v", err)
		}
		if reply.SessionID != "session123" {
			t.Errorf("session_id = %q, want session123", reply.SessionID)
		}
		if reply.Output != "agent response" {
			t.Errorf("output = %q, want %q", reply.Output, "agent response")
		}
		if reply.Error != "" {
			t.Errorf("unexpected error: %q", reply.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task reply")
	}

	// Verify runner received the right args.
	if runner.sessionID != "session123" {
		t.Errorf("runner sessionID = %q", runner.sessionID)
	}
	if runner.input != "hello" {
		t.Errorf("runner input = %q", runner.input)
	}
}

func TestHubClient_MalformedInbox_NoReply(t *testing.T) {
	m := newTestMessenger()
	runner := &stubRunner{output: "should not be called"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		hc := agent.NewHubClient("bot", "1.0", nil, nil, m, runner, nil, nil)
		_ = hc.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send malformed JSON — runner should not be called.
	m.Publish(ctx, "agent.bot.inbox", []byte("not-json")) //nolint:errcheck

	// Brief window: if runner is called we'd see sessionID set.
	time.Sleep(50 * time.Millisecond)
	if runner.sessionID != "" {
		t.Error("runner was called despite malformed envelope")
	}
}

func TestHubClient_StopsOnContextCancel(t *testing.T) {
	m := newTestMessenger()
	runner := &stubRunner{}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		hc := agent.NewHubClient("stopper", "1.0", nil, nil, m, runner, nil, nil)
		done <- hc.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned err: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}
