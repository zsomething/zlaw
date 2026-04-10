// Package session manages multi-session event routing and output broadcasting.
package session

// EventType identifies the kind of event in the newline-delimited JSON bus protocol.
type EventType = string

const (
	// EventUserTurn carries a new message from the user.
	EventUserTurn EventType = "user_turn"
	// EventAssistantDelta carries an incremental token from a streaming LLM response.
	EventAssistantDelta EventType = "assistant_delta"
	// EventAssistantDone signals the end of an assistant turn with the full text.
	EventAssistantDone EventType = "assistant_done"
	// EventToolCall reports a tool invocation.
	EventToolCall EventType = "tool_call"
	// EventToolResult reports a tool response.
	EventToolResult EventType = "tool_result"
	// EventThinking is sent periodically while the agent is processing a turn.
	EventThinking EventType = "thinking"
	// EventError reports an error that occurred during a turn.
	EventError EventType = "error"
	// EventShutdown signals that the daemon is shutting down.
	EventShutdown EventType = "shutdown"
	// EventSubscribe is sent by CLI attach clients to register interest in a session.
	EventSubscribe EventType = "subscribe"
)

// Event is the wire type for the event bus. A single Data field carries the
// payload for all event types (delta text, error message, user input, etc.)
// to keep the protocol simple.
type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"session_id,omitempty"`
	// Data carries the payload: delta text, complete response, user input, or error message.
	Data string `json:"data,omitempty"`
	// Origin identifies which channel submitted the turn that produced this event.
	// Set on EventAssistantDone and EventAssistantDelta. Examples: "telegram", "cli-attach".
	Origin string `json:"origin,omitempty"`
	// Input is the original user message that triggered this turn.
	// Set on EventAssistantDone so sinks can attribute the response to a specific query.
	Input string `json:"input,omitempty"`
}
