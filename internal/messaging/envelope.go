package messaging

// TaskEnvelope is the payload sent to an agent's inbox subject when a task is
// delegated via the hub. The format will be formalised in the A2A card; this
// version carries the minimum needed to run an agent turn and route the reply.
type TaskEnvelope struct {
	// SessionID identifies the conversation session. The receiving agent uses
	// this to look up or create the appropriate history.
	SessionID string `json:"session_id"`

	// Input is the user message / instruction text for the agent turn.
	Input string `json:"input"`

	// ReplyTo is the NATS subject the agent must publish its TaskReply to once
	// the turn completes. Required.
	ReplyTo string `json:"reply_to"`
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
