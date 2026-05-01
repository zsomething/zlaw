package hub_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/zsomething/zlaw/internal/hub"
)

// startTestNATS launches an in-process NATS server for testing and returns a
// connected client connection. The server is shut down when the test ends.
func startTestNATS(t *testing.T) *nats.Conn {
	t.Helper()

	opts := &server.Options{
		Host:   "127.0.0.1",
		Port:   -1, // random port
		NoLog:  true,
		NoSigs: true,
	}
	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("create test NATS server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("test NATS server did not become ready")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		t.Fatalf("connect to test NATS: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func publishReg(t *testing.T, nc *nats.Conn, id, version string, caps []string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{
		"id":           id,
		"version":      version,
		"capabilities": caps,
	})
	if err := nc.Publish("zlaw.registry", data); err != nil {
		t.Fatalf("publish registration: %v", err)
	}
	nc.Flush() //nolint:errcheck
}

func TestRegistry_RegisterAndList(t *testing.T) {
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := hub.NewRegistry(slog.Default())
	if err := reg.Start(ctx, nc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	publishReg(t, nc, "coding", "1.0.0", []string{"bash", "glob"})

	// Give the subscription time to deliver.
	time.Sleep(50 * time.Millisecond)

	entries := reg.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ID != "coding" {
		t.Errorf("name = %q, want %q", e.ID, "coding")
	}
	if e.Status != hub.AgentConnected {
		t.Errorf("status = %q, want connected", e.Status)
	}
	if len(e.Capabilities) != 2 {
		t.Errorf("capabilities = %v, want [bash glob]", e.Capabilities)
	}
}

func TestRegistry_Get(t *testing.T) {
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := hub.NewRegistry(slog.Default())
	if err := reg.Start(ctx, nc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	publishReg(t, nc, "assistant", "2.0.0", []string{"memory_save"})
	time.Sleep(50 * time.Millisecond)

	got, ok := reg.Get("assistant")
	if !ok {
		t.Fatal("Get returned not found")
	}
	if got.Version != "2.0.0" {
		t.Errorf("version = %q, want %q", got.Version, "2.0.0")
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("Get should return false for unknown agent")
	}
}

func TestRegistry_HeartbeatUpdates(t *testing.T) {
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := hub.NewRegistry(slog.Default())
	if err := reg.Start(ctx, nc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	publishReg(t, nc, "worker", "1.0.0", []string{"bash"})
	time.Sleep(50 * time.Millisecond)

	before, _ := reg.Get("worker")

	// Small sleep so the second publish has a later timestamp.
	time.Sleep(5 * time.Millisecond)
	publishReg(t, nc, "worker", "1.0.1", []string{"bash", "glob"})
	time.Sleep(50 * time.Millisecond)

	after, _ := reg.Get("worker")
	if !after.LastHeartbeat.After(before.LastHeartbeat) {
		t.Error("heartbeat timestamp not updated on second publish")
	}
	if after.Version != "1.0.1" {
		t.Errorf("version not updated: got %q", after.Version)
	}
}

func TestRegistry_Deregister(t *testing.T) {
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := hub.NewRegistry(slog.Default())
	if err := reg.Start(ctx, nc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	publishReg(t, nc, "temp", "1.0.0", nil)
	time.Sleep(50 * time.Millisecond)

	if _, ok := reg.Get("temp"); !ok {
		t.Fatal("entry should exist before Deregister")
	}

	reg.Deregister("temp")

	if _, ok := reg.Get("temp"); ok {
		t.Error("entry should be gone after Deregister")
	}
}

func TestRegistry_InvalidMessage(t *testing.T) {
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := hub.NewRegistry(slog.Default())
	if err := reg.Start(ctx, nc); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Publish malformed JSON — should not panic.
	nc.Publish("zlaw.registry", []byte("not-json")) //nolint:errcheck
	nc.Flush()                                      //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	// Publish valid JSON but missing name.
	data, _ := json.Marshal(map[string]any{"version": "1.0.0"})
	nc.Publish("zlaw.registry", data) //nolint:errcheck
	nc.Flush()                        //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	if len(reg.List()) != 0 {
		t.Error("invalid messages should not create registry entries")
	}
}
