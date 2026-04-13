// Package messaging defines the inter-agent messaging contract.
package messaging

import (
	"context"
	"time"
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
}
