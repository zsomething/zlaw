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

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	conn := result.Conn
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

func TestStartNATS_ACLEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{Listen: "127.0.0.1:14523"},
		Agents: []config.AgentEntry{
			{Name: "manager", Manager: true},
			{Name: "specialist"},
			{Name: "specialist2"},
		},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}

	// Verify per-agent tokens were generated.
	if len(result.ACL.AgentTokens) != 3 {
		t.Fatalf("expected 3 agent tokens, got %d", len(result.ACL.AgentTokens))
	}
	if result.ACL.HubToken == "" {
		t.Fatal("expected non-empty hub token")
	}

	natsURL := result.Conn.ConnectedUrl()

	// Specialist can publish to manager inbox (allowed).
	specToken := result.ACL.AgentTokens["specialist"]
	specConn, err := nats.Connect(natsURL, nats.UserInfo("specialist", specToken))
	if err != nil {
		t.Fatalf("specialist connect: %v", err)
	}
	defer specConn.Close()

	if err := specConn.Publish("agent.manager.inbox", []byte("test")); err != nil {
		t.Fatalf("specialist publish to manager.inbox (expected allowed): %v", err)
	}
	if err := specConn.Flush(); err != nil {
		t.Fatalf("flush after allowed publish: %v", err)
	}

	// Specialist attempting to publish to another specialist's inbox must be rejected.
	// The NATS server sends a permissions violation and closes the connection.
	violationCh := make(chan error, 1)
	specConn.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, e error) {
		violationCh <- e
	})

	_ = specConn.Publish("agent.specialist2.inbox", []byte("forbidden"))
	_ = specConn.Flush()

	// The NATS server closes the connection on publish permission violation.
	// Wait briefly for the async error or disconnection.
	select {
	case <-violationCh:
		// permission violation received — ACL enforced correctly
	case <-time.After(2 * time.Second):
		// If no error: check whether the connection was closed by the server.
		if specConn.IsConnected() {
			t.Error("specialist published to specialist2.inbox without a permission violation")
		}
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
