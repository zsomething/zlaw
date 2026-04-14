package hub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Default stream constants.
const (
	// AgentInboxStream is the JetStream stream name for agent inbox messages.
	AgentInboxStream = "AGENT_INBOX"

	// AgentInboxSubjects is the NATS subject pattern for agent inbox messages.
	AgentInboxSubjects = "agent.>"

	// DefaultStreamMaxAge is the default message retention window (1 hour).
	DefaultStreamMaxAge = time.Hour
)

// StreamManager handles JetStream stream lifecycle operations.
type StreamManager struct {
	conn *nats.Conn
}

// NewStreamManager creates a StreamManager backed by nc.
func NewStreamManager(nc *nats.Conn) *StreamManager {
	return &StreamManager{conn: nc}
}

// EnsureAgentInboxStream creates the AGENT_INBOX stream if it does not exist.
// It is idempotent — if the stream already exists, the call is a no-op.
func (sm *StreamManager) EnsureAgentInboxStream(ctx context.Context, maxAge time.Duration) error {
	if maxAge <= 0 {
		maxAge = DefaultStreamMaxAge
	}

	js, err := sm.conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream: get context: %w", err)
	}

	// Check if stream already exists (idempotency).
	_, err = js.StreamInfo(AgentInboxStream)
	if err == nil {
		// Stream exists, nothing to do.
		return nil
	}

	// Any other error besides "not found" is unexpected.
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("jetstream: stream info: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      AgentInboxStream,
		Subjects:  []string{AgentInboxSubjects},
		Storage:   nats.FileStorage,
		Retention: nats.WorkQueuePolicy,
		MaxAge:    maxAge,
	})
	if err != nil {
		return fmt.Errorf("jetstream: create stream %s: %w", AgentInboxStream, err)
	}

	return nil
}

// compile-time interface check
var _ StreamManagerer = (*StreamManager)(nil)

// StreamManagerer is the interface for stream management operations.
type StreamManagerer interface {
	EnsureAgentInboxStream(ctx context.Context, maxAge time.Duration) error
}
