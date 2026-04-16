package builtin_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/messaging"
	"github.com/zsomething/zlaw/internal/tools/builtin"
)

// staticRegistry always reports a fixed set of registered agents.
type staticRegistry struct{ ids map[string]bool }

func newStaticRegistry(ids ...string) *staticRegistry {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return &staticRegistry{ids: m}
}

func (r *staticRegistry) IsRegistered(id string) bool { return r.ids[id] }

// respondingMessenger is a ChanMessenger that, when it receives on a known
// inbox subject, publishes a canned TaskReply to the ReplyTo in the envelope.
type respondingMessenger struct {
	*messaging.ChanMessenger
	replyOutput string
}

func newRespondingMessenger(inboxSubject, replyOutput string) *respondingMessenger {
	cm := messaging.NewChanMessenger()
	rm := &respondingMessenger{ChanMessenger: cm, replyOutput: replyOutput}
	ctx := context.Background()
	_, _ = cm.Subscribe(ctx, inboxSubject, func(data []byte) {
		// Extract reply subject from envelope's reply_to field.
		var env messaging.TaskEnvelope
		if err := json.Unmarshal(data, &env); err != nil || env.ReplyTo == "" {
			return
		}
		reply := messaging.TaskReply{
			SessionID: env.SessionID,
			Output:    replyOutput,
		}
		replyData, _ := json.Marshal(reply)
		_ = cm.Publish(ctx, env.ReplyTo, replyData)
	})
	return rm
}

func TestAgentDelegate_StandaloneMode(t *testing.T) {
	tool := &builtin.AgentDelegate{AgentID: "manager"}
	input, _ := json.Marshal(map[string]any{"id": "worker", "task": "do something"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "standalone mode") {
		t.Errorf("expected standalone mode error, got %v", err)
	}
}

func TestAgentDelegate_AgentNotRegistered(t *testing.T) {
	cm := messaging.NewChanMessenger()
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("other-agent"),
	}
	input, _ := json.Marshal(map[string]any{"id": "worker", "task": "do something"})
	_, err := tool.Execute(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected not registered error, got %v", err)
	}
}

func TestAgentDelegate_Success(t *testing.T) {
	rm := newRespondingMessenger("agent.worker.inbox", "task completed")
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: rm,
		Registry:  newStaticRegistry("worker"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	input, _ := json.Marshal(map[string]any{
		"id":   "worker",
		"task": "do something",
		"context": map[string]any{
			"priority": "high",
		},
	})
	output, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "task completed" {
		t.Errorf("output = %q, want %q", output, "task completed")
	}
}

func TestAgentDelegate_TargetReturnsError(t *testing.T) {
	cm := messaging.NewChanMessenger()
	ctx := context.Background()
	_, _ = cm.Subscribe(ctx, "agent.worker.inbox", func(data []byte) {
		var env messaging.TaskEnvelope
		if err := json.Unmarshal(data, &env); err != nil || env.ReplyTo == "" {
			return
		}
		reply := messaging.TaskReply{
			SessionID: env.SessionID,
			Error:     "worker failed to process task",
		}
		replyData, _ := json.Marshal(reply)
		_ = cm.Publish(ctx, env.ReplyTo, replyData)
	})

	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}

	input, _ := json.Marshal(map[string]any{"id": "worker", "task": "do something"})
	_, err := tool.Execute(ctx, input)
	if err == nil || !strings.Contains(err.Error(), "worker failed to process task") {
		t.Errorf("expected worker error in result, got %v", err)
	}
}

func TestAgentDelegate_NoRegistry_Succeeds(t *testing.T) {
	// When Registry is nil, validation is skipped and the tool sends regardless.
	rm := newRespondingMessenger("agent.worker.inbox", "ok")
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: rm,
		Registry:  nil, // no registry = skip validation
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	input, _ := json.Marshal(map[string]any{"id": "worker", "task": "ping"})
	output, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "ok" {
		t.Errorf("output = %q, want ok", output)
	}
}

func TestAgentDelegate_Timeout(t *testing.T) {
	// Messenger receives but never replies — delegate should time out.
	cm := messaging.NewChanMessenger()
	ctx := context.Background()
	_, _ = cm.Subscribe(ctx, "agent.worker.inbox", func(data []byte) {
		// Never reply — simulate a hung agent.
	})

	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}

	// Use a short timeout for the test.
	toolExecute := func(ctx context.Context) (string, error) {
		return tool.Execute(ctx, json.RawMessage(`{"id":"worker","task":"do it"}`))
	}

	// The delegate tool uses its own 60s timeout internally, so we test with
	// a context that's cancelled while waiting for reply.
	shortCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	_, err := toolExecute(shortCtx)
	if err == nil {
		t.Error("expected error, got nil")
	}
	// Should be a context deadline exceeded error.
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "context") {
		t.Errorf("expected timeout/context error, got: %v", err)
	}
}

func TestAgentDelegate_ContextCancelled(t *testing.T) {
	// Parent context cancelled mid-delegation — the delegate should return
	// with the context error rather than waiting for a reply.
	cm := messaging.NewChanMessenger()
	// Buffered so the subscription handler never blocks (simulates a slow target).
	ctx := context.Background()
	_, _ = cm.Subscribe(ctx, "agent.worker.inbox", func(data []byte) {
		// Deliberately never reply — simulate a slow/stuck target agent.
		// The handler returns immediately; the tool will time out waiting for reply.
	})

	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}

	parentCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := tool.Execute(parentCtx, json.RawMessage(`{"id":"worker","task":"do it"}`))
		done <- err
	}()

	// Give the goroutine time to enter the reply wait.
	time.Sleep(100 * time.Millisecond)
	// Cancel while reply is still pending.
	cancel()

	select {
	case err := <-done:
		// Cancelled context should produce an error (timeout or context cancelled).
		if err == nil {
			t.Error("expected error from cancelled context, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("delegate did not return after context cancellation")
	}
}

func TestAgentDelegate_InvalidInput_MissingID(t *testing.T) {
	cm := messaging.NewChanMessenger()
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"task":"do it"}`))
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected 'id is required' error, got: %v", err)
	}
}

func TestAgentDelegate_InvalidInput_MissingTask(t *testing.T) {
	cm := messaging.NewChanMessenger()
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"worker"}`))
	if err == nil || !strings.Contains(err.Error(), "task is required") {
		t.Errorf("expected 'task is required' error, got: %v", err)
	}
}

func TestAgentDelegate_InvalidInput_NotJSON(t *testing.T) {
	cm := messaging.NewChanMessenger()
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: cm,
		Registry:  newStaticRegistry("worker"),
	}
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil || !strings.Contains(err.Error(), "invalid input") {
		t.Errorf("expected 'invalid input' error, got: %v", err)
	}
}

func TestAgentDelegate_SelfDelegation(t *testing.T) {
	// Delegating to self should succeed (no special handling needed).
	rm := newRespondingMessenger("agent.manager.inbox", "self-reply")
	tool := &builtin.AgentDelegate{
		AgentID:   "manager",
		Messenger: rm,
		Registry:  newStaticRegistry("manager"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := tool.Execute(ctx, json.RawMessage(`{"id":"manager","task":"reflect"}`))
	if err != nil {
		t.Errorf("self-delegation should not error, got: %v", err)
	}
}
