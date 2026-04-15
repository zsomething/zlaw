package messaging

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSMessenger is the production Messenger implementation backed by nats.Conn.
// Connect via NewNATSMessenger; the caller owns the connection lifecycle.
type NATSMessenger struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

// NewNATSMessenger connects to the NATS server at url and returns a NATSMessenger.
// agentName and token are used for authentication when non-empty; the NATS server
// must have been configured with a matching username/password entry.
func NewNATSMessenger(url, agentName, token string) (*NATSMessenger, error) {
	var opts []nats.Option
	if agentName != "" && token != "" {
		opts = append(opts, nats.UserInfo(agentName, token))
	}
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect %s: %w", url, err)
	}
	// Attempt to get JetStream context (nil if server has no JetStream license).
	js, _ := conn.JetStream()
	return &NATSMessenger{conn: conn, js: js}, nil
}

// Close drains and closes the underlying NATS connection.
func (m *NATSMessenger) Close() {
	_ = m.conn.Drain()
}

func (m *NATSMessenger) Publish(_ context.Context, subject string, payload []byte) error {
	return m.conn.Publish(subject, payload)
}

func (m *NATSMessenger) Subscribe(_ context.Context, subject string, handler func([]byte)) (Subscription, error) {
	sub, err := m.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (m *NATSMessenger) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	msg, err := m.conn.RequestWithContext(ctx, subject, payload)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

// JetStream returns the JetStream context, or nil if JetStream is not available.
func (m *NATSMessenger) JetStream() JetStreamer {
	if m.js == nil {
		return nil
	}
	return &jetStreamAdapter{js: m.js}
}

// jetStreamAdapter adapts nats.JetStreamContext to messaging.JetStreamer.
type jetStreamAdapter struct {
	js nats.JetStreamContext
}

func (a *jetStreamAdapter) Fetch(ctx context.Context, consumer string, stream string, handler func(*JetMsg)) error {
	sub, err := a.js.PullSubscribe("", consumer,
		nats.AckExplicit(),
		nats.Bind(stream, consumer),
	)
	if err != nil {
		return fmt.Errorf("pull subscribe: %w", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	msgs, err := sub.Fetch(1, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	for _, msg := range msgs {
		handler(&JetMsg{msg: msg})
	}
	return nil
}

func (a *jetStreamAdapter) FetchOnSubject(ctx context.Context, consumer string, stream string, subject string, handler func(*JetMsg)) error {
	sub, err := a.js.PullSubscribe(subject, consumer,
		nats.AckExplicit(),
		nats.Bind(stream, consumer),
	)
	if err != nil {
		return fmt.Errorf("pull subscribe: %w", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	msgs, err := sub.Fetch(1, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	for _, msg := range msgs {
		handler(&JetMsg{msg: msg})
	}
	return nil
}

func (a *jetStreamAdapter) CreatePullConsumer(ctx context.Context, consumer string, stream string, filter string) error {
	cfg := &nats.ConsumerConfig{
		Name:          consumer,
		Durable:       consumer,
		FilterSubject: filter,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    10,
	}
	_, err := a.js.AddConsumer(stream, cfg)
	if err != nil && !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
		return fmt.Errorf("add consumer: %w", err)
	}
	return nil
}

// compile-time interface check
var _ Messenger = (*NATSMessenger)(nil)
