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
