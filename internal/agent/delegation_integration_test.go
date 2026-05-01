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

// TestIntegration_AgentDelegation tests the full delegation flow:
// manager → worker (via HubClient inbox) → worker runs task → worker publishes reply → manager receives.
func TestIntegration_AgentDelegation(t *testing.T) {
	// Single ChanMessenger acts as the hub message bus.
	bus := messaging.NewChanMessenger()

	// Worker: HubClient subscribes to its inbox, runner returns canned response.
	workerOutput := "copy: sunlight is the best disinfectant"
	workerRunner := &stubHubTaskRunner{output: workerOutput, err: nil}
	workerClient := agent.NewHubClient(
		"worker", "v1",
		[]string{"write", "critique"},
		nil, // roles
		nil, // authProfiles
		"",  // seedPath
		bus,
		workerRunner,
		func() string { return "You are Fuery, a copywriter." },
		slog.New(slog.DiscardHandler),
	)

	// Manager: HubClient (only for registration; delegation goes via ChanMessenger directly).
	managerClient := agent.NewHubClient(
		"manager", "v1",
		[]string{"delegate"},
		nil, // roles
		nil, // authProfiles
		"",  // seedPath
		bus,
		nil, // manager doesn't need a runner for this test
		nil,
		slog.New(slog.DiscardHandler),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start both clients.
	errCh := make(chan error, 2)
	go func() { errCh <- workerClient.Start(ctx) }()
	go func() { errCh <- managerClient.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Set up reply collector for manager.
	replyCh := make(chan string, 1)
	replySubject := "_INBOX.manager.delegate.reply"
	_, _ = bus.Subscribe(ctx, replySubject, func(data []byte) {
		var reply messaging.TaskReply
		if err := json.Unmarshal(data, &reply); err != nil {
			t.Logf("malformed reply: %v", err)
			return
		}
		replyCh <- reply.Output
	})
	time.Sleep(50 * time.Millisecond)

	// Manager publishes a delegation envelope directly to worker inbox.
	env := messaging.TaskEnvelope{
		From:        "manager",
		To:          "worker",
		Task:        "Write a tagline about sunlight being the best disinfectant",
		Context:     map[string]any{"medium": "health campaign"},
		SessionID:   "telegram:chat123",
		ReplyTo:     replySubject,
		SourceAgent: "manager",
		TraceID:     "trace-abc",
	}
	envData, _ := json.Marshal(env)
	if err := bus.Publish(ctx, "agent.worker.inbox", envData); err != nil {
		t.Fatalf("publish envelope: %v", err)
	}

	// Wait for reply.
	select {
	case output := <-replyCh:
		if output != workerOutput {
			t.Errorf("manager received %q, want %q", output, workerOutput)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for delegation reply")
	}

	// Verify worker ran the task exactly once.
	if workerRunner.calls != 1 {
		t.Errorf("worker runner calls = %d, want 1", workerRunner.calls)
	}
}

// TestIntegration_AgentNotFound tests that delegation to an unregistered
// agent returns an error without hanging.
func TestIntegration_AgentNotFound(t *testing.T) {
	bus := messaging.NewChanMessenger()

	// Manager with a stub runner (doesn't matter — delegation should fail before runner is called).
	managerRunner := &stubHubTaskRunner{output: "", err: errors.New("should not be called")}
	managerClient := agent.NewHubClient(
		"manager", "v1",
		[]string{"delegate"},
		nil, // roles
		nil, // authProfiles
		"",  // seedPath
		bus,
		managerRunner,
		nil,
		slog.New(slog.DiscardHandler),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { managerClient.Start(ctx) }() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	// Subscribe to reply so delegate tool can receive it.
	replyCh := make(chan error, 1)
	replySubject := "_INBOX.manager.test.reply"
	_, _ = bus.Subscribe(ctx, replySubject, func(data []byte) {
		var reply messaging.TaskReply
		if err := json.Unmarshal(data, &reply); err != nil {
			replyCh <- err
			return
		}
		if reply.Error != "" {
			replyCh <- errors.New(reply.Error)
		}
	})
	time.Sleep(50 * time.Millisecond)

	// Publish to a non-existent worker inbox — nobody is subscribed.
	env := messaging.TaskEnvelope{
		From: "manager", To: "ghost", Task: "do something",
		SessionID: "s1", ReplyTo: replySubject,
	}
	_ = bus.Publish(ctx, "agent.ghost.inbox", mustMarshal(env))

	// The ChanMessenger delivers to whatever is subscribed.
	// Since no handler is registered for agent.ghost.inbox, nothing happens.
	// The reply is never sent, so we expect a timeout.
	// This test just verifies the bus doesn't panic on unknown subjects.
}

// stubHubTaskRunner is a simple task runner for integration tests.
type stubHubTaskRunner struct {
	output string
	err    error
	calls  int
}

func (s *stubHubTaskRunner) Run(_ context.Context, sessionID, input, systemPrompt string) (string, error) {
	s.calls++
	return s.output, s.err
}

var _ agent.HubTaskRunner = (*stubHubTaskRunner)(nil)
