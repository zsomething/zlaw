package messaging

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSMessenger is the production Messenger implementation backed by nats.Conn.
// Connect via NewNATSMessenger; the caller owns the connection lifecycle.
type NATSMessenger struct {
	conn *nats.Conn
}

// NewNATSMessenger connects to the NATS server at url and returns a NATSMessenger.
func NewNATSMessenger(url string) (*NATSMessenger, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect %s: %w", url, err)
	}
	return &NATSMessenger{conn: conn}, nil
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

// compile-time interface check
var _ Messenger = (*NATSMessenger)(nil)
