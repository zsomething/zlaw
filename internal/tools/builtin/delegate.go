package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zsomething/zlaw/internal/ctxkey"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/messaging"
)

const delegateTimeout = 60 * time.Second

// AgentLookup checks whether an agent ID is currently registered in the hub.
type AgentLookup interface {
	IsRegistered(id string) bool
}

// AgentDelegate is a builtin tool that delegates a task to another agent via
// the hub. It builds a TaskEnvelope with an explicit ReplyTo subject, publishes
// it to the target agent's inbox, and waits for a TaskReply on that subject.
//
// The request-reply cycle is managed explicitly (Subscribe → Publish → wait)
// rather than via Messenger.Request, because the reply subject must appear in
// the envelope JSON so the receiving agent knows where to publish its reply.
//
// If Messenger is nil (standalone mode) the tool returns an error immediately.
// If Registry is non-nil and the target agent is not registered, the tool
// returns an error without sending anything.
type AgentDelegate struct {
	// AgentID is the ID of this (delegating) agent, used to populate From.
	AgentID string

	// Messenger is used to send the TaskEnvelope and await the reply.
	// Nil means the agent is running in standalone mode (no hub).
	Messenger messaging.Messenger

	// Registry is used to validate that the target agent is registered.
	// When nil, registry validation is skipped.
	Registry AgentLookup
}

var agentDelegateSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "The ID of the target agent to delegate the task to."
    },
    "task": {
      "type": "string",
      "description": "The instruction or question to send to the target agent."
    },
    "context": {
      "type": "object",
      "description": "Optional structured metadata to pass alongside the task.",
      "additionalProperties": true
    }
  },
  "required": ["id", "task"]
}`)

func (*AgentDelegate) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name: "agent_delegate",
		Description: "Delegate a task to another agent connected to the hub. " +
			"The target agent runs the task and returns its response. " +
			"Returns an error in standalone mode (not connected to hub) or when " +
			"the target agent is not registered.",
		InputSchema: agentDelegateSchema,
	}
}

type agentDelegateInput struct {
	ID      string         `json:"id"`
	Task    string         `json:"task"`
	Context map[string]any `json:"context,omitempty"`
}

func (d *AgentDelegate) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if d.Messenger == nil {
		return "", fmt.Errorf("agent_delegate: not connected to hub (standalone mode)")
	}

	var in agentDelegateInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("agent_delegate: invalid input: %w", err)
	}
	if in.ID == "" {
		return "", fmt.Errorf("agent_delegate: id is required")
	}
	if in.Task == "" {
		return "", fmt.Errorf("agent_delegate: task is required")
	}

	if d.Registry != nil && !d.Registry.IsRegistered(in.ID) {
		return "", fmt.Errorf("agent_delegate: agent %q is not registered", in.ID)
	}

	sessionID := ctxkey.SessionIDFrom(ctx)
	traceID := ctxkey.TraceIDOf(ctx)
	sourceChannel := ctxkey.SourceChannelOf(ctx)

	// Generate a unique reply subject and subscribe before publishing, so we
	// don't miss the reply.
	replySubject := fmt.Sprintf("_INBOX.delegate.%s.%d", d.AgentID, time.Now().UnixNano())
	replyCh := make(chan []byte, 1)
	sub, err := d.Messenger.Subscribe(ctx, replySubject, func(data []byte) {
		select {
		case replyCh <- data:
		default:
		}
	})
	if err != nil {
		return "", fmt.Errorf("agent_delegate: subscribe reply inbox: %w", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	// Build session context for the delegation.
	sessionCtx := map[string]any{}
	if traceID != "" {
		sessionCtx["trace_id"] = traceID
	}
	if sourceChannel != "" {
		sessionCtx["originating_channel"] = sourceChannel
	}

	env := messaging.TaskEnvelope{
		From:           d.AgentID,
		To:             in.ID,
		Task:           in.Task,
		Context:        in.Context,
		SessionID:      sessionID,
		ReplyTo:        replySubject,
		SourceAgent:    d.AgentID,
		SessionContext: sessionCtx,
		TraceID:        traceID,
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("agent_delegate: marshal envelope: %w", err)
	}

	inboxSubject := fmt.Sprintf("agent.%s.inbox", in.ID)
	if err := d.Messenger.Publish(ctx, inboxSubject, payload); err != nil {
		return "", fmt.Errorf("agent_delegate: publish to %q: %w", in.ID, err)
	}

	tctx, cancel := context.WithTimeout(ctx, delegateTimeout)
	defer cancel()

	select {
	case replyData := <-replyCh:
		var reply messaging.TaskReply
		if err := json.Unmarshal(replyData, &reply); err != nil {
			return "", fmt.Errorf("agent_delegate: malformed reply from %q: %w", in.ID, err)
		}
		if reply.Error != "" {
			return "", fmt.Errorf("agent_delegate: agent %q returned error: %s", in.ID, reply.Error)
		}
		return reply.Output, nil
	case <-tctx.Done():
		return "", fmt.Errorf("agent_delegate: timeout waiting for reply from %q", in.ID)
	}
}
