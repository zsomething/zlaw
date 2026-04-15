package hub_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/messaging"
)

func TestNewAuditLogger_DisabledWhenEmptyPath(t *testing.T) {
	al, err := hub.NewAuditLogger("", nil, nil)
	if al != nil {
		t.Error("expected nil AuditLogger when path is empty")
	}
	if err != nil {
		t.Error("expected no error when path is empty")
	}
}

func TestNewAuditLogger_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")

	cm := &stubMessenger{}
	al, err := hub.NewAuditLogger(auditPath, cm, nil)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	if al == nil {
		t.Fatal("expected non-nil AuditLogger")
	}
	defer al.Close()

	// File should exist.
	if _, err := os.Stat(auditPath); err != nil {
		t.Errorf("audit file not created at %s: %v", auditPath, err)
	}
}

func TestAuditLogger_LogEvent_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")

	cm := &stubMessenger{}
	al, err := hub.NewAuditLogger(auditPath, cm, nil)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}
	defer al.Close()

	entry := hub.AuditEntry{
		Type:      hub.AuditEventMgmt,
		From:      "hub",
		Subject:   "test",
		Direction: "out",
		Extra:     map[string]any{"op": "agent.create"},
	}
	al.LogEvent(entry)

	// Read the file and verify it's valid JSON.
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit file: %v", err)
	}

	var parsed hub.AuditEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("audit file is not valid JSON: %v", err)
	}
	if parsed.Type != hub.AuditEventMgmt {
		t.Errorf("Type = %q, want %q", parsed.Type, hub.AuditEventMgmt)
	}
	if parsed.From != "hub" {
		t.Errorf("From = %q, want %q", parsed.From, "hub")
	}
	if parsed.Timestamp == "" {
		t.Error("Timestamp should be set")
	}
}

func TestAuditLogger_Close(t *testing.T) {
	dir := t.TempDir()
	auditPath := filepath.Join(dir, "audit.jsonl")

	cm := &stubMessenger{}
	al, err := hub.NewAuditLogger(auditPath, cm, nil)
	if err != nil {
		t.Fatalf("NewAuditLogger: %v", err)
	}

	if err := al.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// stubMessenger is a minimal messaging.Messenger for tests.
type stubMessenger struct{}

func (s *stubMessenger) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (s *stubMessenger) Subscribe(_ context.Context, subject string, handler func([]byte)) (messaging.Subscription, error) {
	return &stubSub{subject: subject, handler: handler}, nil
}

func (s *stubMessenger) Request(_ context.Context, subject string, _ []byte, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (s *stubMessenger) JetStream() messaging.JetStreamer { return nil }

type stubSub struct {
	subject string
	handler func([]byte)
}

func (s *stubSub) Unsubscribe() error { return nil }
