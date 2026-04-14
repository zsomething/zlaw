// Package messaging defines the inter-agent messaging contract.
package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// Subscription represents an active subject subscription.
type Subscription interface {
	Unsubscribe() error
}

// Messenger is the inter-agent messaging interface.
// Production code uses NATSMessenger; tests use ChanMessenger.
// The agent initialises Messenger as nil when ZLAW_NATS_URL is unset
// (standalone mode). Hub-dependent operations must check for nil.
type Messenger interface {
	// Publish sends payload to subject with fire-and-forget semantics.
	Publish(ctx context.Context, subject string, payload []byte) error

	// Subscribe registers handler for messages arriving on subject.
	// The returned Subscription must be unsubscribed when done.
	Subscribe(ctx context.Context, subject string, handler func([]byte)) (Subscription, error)

	// Request sends payload to subject and waits up to timeout for a reply.
	Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error)

	// JetStream returns a JetStream context for durable stream operations.
	// Returns nil when JetStream is not available on the server.
	JetStream() JetStreamer
}

// JetStreamer abstracts nats.JetStream for testing and future middleware.
type JetStreamer interface {
	// Fetch pulls messages from a pull consumer, blocking until at least one
	// message arrives or the context deadline is reached. handler is called for
	// each delivered message and must Ack() or Nak() the JetMsg.
	Fetch(ctx context.Context, consumer string, stream string, handler func(*JetMsg)) error

	// CreatePullConsumer creates a durable pull consumer on stream named consumer.
	// If it already exists, CreatePullConsumer is a no-op.
	CreatePullConsumer(ctx context.Context, consumer string, stream string, filter string) error
}

// JetMsg wraps a JetStream message with acknowledgment semantics.
type JetMsg struct {
	msg *nats.Msg
}

// Data returns the raw message payload, or nil if msg is nil.
func (m *JetMsg) Data() []byte {
	if m.msg == nil {
		return nil
	}
	return m.msg.Data
}

func (m *JetMsg) Ack() error {
	if m.msg == nil {
		return fmt.Errorf("ack: nil message")
	}
	return m.msg.Ack()
}

func (m *JetMsg) Nak() error {
	if m.msg == nil {
		return fmt.Errorf("nak: nil message")
	}
	return m.msg.Nak()
}
