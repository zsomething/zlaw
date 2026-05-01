package messaging

import (
	"fmt"

	"github.com/zsomething/zlaw/internal/identity"
)

// TaskEnvelope is the wire format for inter-agent task delegation over the hub.
// From and To carry the stable agent ID (not display name). The receiving agent
// uses SessionID to look up or create the appropriate history, runs the Task as
// an agent turn, and publishes a TaskReply to ReplyTo when done.
//
// Sub-agents receiving a delegation create a fresh session (ignoring SessionID)
// so each delegation is processed independently. SourceAgent and SessionContext
// are provided for logging, auditing, and context propagation.
type TaskEnvelope struct {
	// From is the agent ID of the delegating agent.
	From string `json:"from"`

	// To is the agent ID of the target agent.
	To string `json:"to"`

	// Task is the instruction text for the agent turn.
	Task string `json:"task"`

	// Context holds optional structured metadata passed from the delegating agent.
	Context map[string]any `json:"context,omitempty"`

	// ReplyTo is the NATS subject the receiving agent must publish its TaskReply
	// to once the turn completes. Required.
	ReplyTo string `json:"reply_to"`

	// SessionID identifies the originating conversation session for correlation
	// in TaskReply. The receiving agent creates a fresh session per delegation
	// and ignores this field for history management.
	SessionID string `json:"session_id"`

	// SourceAgent is the delegating agent's ID, used for logging and ACL
	// decisions on the receiving end. Mirrors From for clarity in hub logs.
	SourceAgent string `json:"source_agent,omitempty"`

	// SessionContext holds additional metadata about the originating session
	// (e.g., originating_channel, user_id, trace_id). Passed through for
	// logging and audit purposes; the agent may use it to enrich the task.
	SessionContext map[string]any `json:"session_context,omitempty"`

	// TraceID propagates a distributed trace identifier across agent hops.
	TraceID string `json:"trace_id,omitempty"`

	// Signature is a base64-encoded Ed25519 signature of the envelope's
	// payload (From+To+Task+SessionID). Verified by the receiving agent
	// against the sender's public key from the registry.
	Signature string `json:"signature,omitempty"`
}

// Sign signs this envelope with the given seed.
// The signed payload is From+To+Task+SessionID.
func (e *TaskEnvelope) Sign(seed []byte) error {
	payload := e.From + e.To + e.Task + e.SessionID
	sig, err := identity.Sign(seed, []byte(payload))
	if err != nil {
		return fmt.Errorf("sign envelope: %w", err)
	}
	e.Signature = sig
	return nil
}

// Verify checks the signature against the sender's public key.
// Returns true if the signature is valid, false otherwise.
// An empty signature is considered invalid.
func (e *TaskEnvelope) Verify(publicKey string) bool {
	if e.Signature == "" {
		return false
	}
	payload := e.From + e.To + e.Task + e.SessionID
	return identity.Verify(publicKey, []byte(payload), e.Signature)
}

// TaskReply is the response published back to ReplyTo after the agent turn
// completes.
type TaskReply struct {
	// SessionID mirrors the envelope's SessionID for correlation.
	SessionID string `json:"session_id"`

	// Output is the agent's response text on success.
	Output string `json:"output,omitempty"`

	// Error is a human-readable error description when the turn failed.
	Error string `json:"error,omitempty"`
}
