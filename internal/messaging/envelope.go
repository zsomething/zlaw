package messaging

// TaskEnvelope is the wire format for inter-agent task delegation over the hub.
// From and To carry the stable agent ID (not display name). The receiving agent
// uses SessionID to look up or create the appropriate history, runs the Task as
// an agent turn, and publishes a TaskReply to ReplyTo when done.
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

	// SessionID identifies the conversation session.
	SessionID string `json:"session_id"`

	// TraceID propagates a distributed trace identifier across agent hops.
	TraceID string `json:"trace_id,omitempty"`
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
