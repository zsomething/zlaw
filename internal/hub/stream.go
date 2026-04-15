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

	// inboxSubjectFmt is the agent-specific inbox subject pattern.
	inboxSubjectFmt = "agent.%s.inbox"
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

// EnsureAgentConsumers creates durable pull consumers for each agent name in
// agentNames. The consumer name equals the agent name, filtering on its inbox
// subject (agent.<name>.inbox). It is idempotent — existing consumers are left
// untouched.
func (sm *StreamManager) EnsureAgentConsumers(ctx context.Context, agentNames []string) error {
	js, err := sm.conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream: get context: %w", err)
	}

	for _, name := range agentNames {
		consumer := name
		filter := fmt.Sprintf(inboxSubjectFmt, name)

		cfg := &nats.ConsumerConfig{
			Name:          consumer,
			Durable:       consumer,
			FilterSubject: filter,
			AckPolicy:     nats.AckExplicitPolicy,
			AckWait:       30 * time.Second,
			MaxDeliver:    10,
		}

		_, err := js.AddConsumer(AgentInboxStream, cfg)
		if err != nil && !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return fmt.Errorf("jetstream: create consumer %q: %w", consumer, err)
		}
	}
	return nil
}

// compile-time interface check
var _ StreamManagerer = (*StreamManager)(nil)

// StreamManagerer is the interface for stream management operations.
type StreamManagerer interface {
	EnsureAgentInboxStream(ctx context.Context, maxAge time.Duration) error
	EnsureAgentConsumers(ctx context.Context, agentNames []string) error
}
