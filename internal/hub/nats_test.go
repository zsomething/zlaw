package hub_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

func TestStartNATS_Embedded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{Listen: "127.0.0.1:14522"},
	}

	conn, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer conn.Close()

	if !conn.IsConnected() {
		t.Fatal("expected connection to be active")
	}

	// Verify basic pub/sub over the embedded server.
	ch := make(chan []byte, 1)
	sub, err := conn.Subscribe("test.subject", func(msg *nats.Msg) {
		ch <- msg.Data
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	if err := conn.Publish("test.subject", []byte("hello")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != "hello" {
			t.Fatalf("got %q, want %q", data, "hello")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestStartNATS_InvalidAddress(t *testing.T) {
	ctx := context.Background()
	cfg := config.HubConfig{
		NATS: config.NATSConfig{Listen: "notavalidaddress"},
	}
	_, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err == nil {
		t.Fatal("expected error for invalid listen address")
	}
}
