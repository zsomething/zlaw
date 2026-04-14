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

func TestStreamManager_EnsureAgentInboxStream_Idempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{
			Listen:    "127.0.0.1:14540",
			JetStream: true,
		},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer result.Conn.Close()

	sm := hub.NewStreamManager(result.Conn)

	// First call creates the stream.
	if err := sm.EnsureAgentInboxStream(ctx, 0); err != nil {
		t.Fatalf("EnsureAgentInboxStream (first): %v", err)
	}

	// Second call is a no-op — should not error.
	if err := sm.EnsureAgentInboxStream(ctx, 0); err != nil {
		t.Fatalf("EnsureAgentInboxStream (second/idempotent): %v", err)
	}

	// Verify stream exists by checking stream info via JetStream API.
	js, err := result.Conn.JetStream()
	if err != nil {
		t.Fatalf("JetStream context: %v", err)
	}
	info, err := js.StreamInfo(hub.AgentInboxStream)
	if err != nil {
		t.Fatalf("StreamInfo(%s): %v", hub.AgentInboxStream, err)
	}
	if info.Config.Name != hub.AgentInboxStream {
		t.Errorf("stream name = %q, want %q", info.Config.Name, hub.AgentInboxStream)
	}
	if len(info.Config.Subjects) != 1 || info.Config.Subjects[0] != hub.AgentInboxSubjects {
		t.Errorf("subjects = %v, want [%s]", info.Config.Subjects, hub.AgentInboxSubjects)
	}
	if info.Config.Storage != nats.FileStorage {
		t.Errorf("storage = %v, want FileStorage", info.Config.Storage)
	}
	if info.Config.Retention != nats.WorkQueuePolicy {
		t.Errorf("retention = %v, want WorkQueuePolicy", info.Config.Retention)
	}
}

func TestStreamManager_EnsureAgentInboxStream_NonDefaultMaxAge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{
			Listen:    "127.0.0.1:14541",
			JetStream: true,
		},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer result.Conn.Close()

	sm := hub.NewStreamManager(result.Conn)
	wantAge := 2 * time.Hour

	// Delete stream first if it exists from prior tests sharing the store dir.
	js, _ := result.Conn.JetStream()
	_ = js.DeleteStream(hub.AgentInboxStream)

	if err := sm.EnsureAgentInboxStream(ctx, wantAge); err != nil {
		t.Fatalf("EnsureAgentInboxStream: %v", err)
	}

	info, err := js.StreamInfo(hub.AgentInboxStream)
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.Config.MaxAge != wantAge {
		t.Errorf("MaxAge = %v, want %v", info.Config.MaxAge, wantAge)
	}
}

func TestStreamManager_EnsureAgentInboxStream_ExternalNATS(t *testing.T) {
	ctx := context.Background()
	cfg := config.HubConfig{
		NATS: config.NATSConfig{Listen: "127.0.0.1:14542"},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer result.Conn.Close()

	// External connection has no JetStream; js.StreamInfo should error.
	sm := hub.NewStreamManager(result.Conn)
	err = sm.EnsureAgentInboxStream(ctx, 0)
	if err == nil {
		t.Error("expected error when JetStream is not available")
	}
}
