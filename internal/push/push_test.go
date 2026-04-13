package push_test

import (
	"context"
	"testing"

	"github.com/zsomething/zlaw/internal/push"
)

type stubPusher struct {
	gotAddress string
	gotMessage string
}

func (s *stubPusher) Push(_ context.Context, address string, message string) error {
	s.gotAddress = address
	s.gotMessage = message
	return nil
}

func TestRegistry_Push(t *testing.T) {
	r := push.NewRegistry()
	stub := &stubPusher{}
	r.Register("telegram", stub)

	err := r.Push(context.Background(), "telegram:123456789", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stub.gotAddress != "123456789" {
		t.Errorf("address: got %q, want %q", stub.gotAddress, "123456789")
	}
	if stub.gotMessage != "hello" {
		t.Errorf("message: got %q, want %q", stub.gotMessage, "hello")
	}
}

func TestRegistry_Push_InvalidTarget(t *testing.T) {
	r := push.NewRegistry()
	err := r.Push(context.Background(), "notarget", "msg")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
}

func TestRegistry_Push_UnknownAdapter(t *testing.T) {
	r := push.NewRegistry()
	err := r.Push(context.Background(), "slack:C123", "msg")
	if err == nil {
		t.Fatal("expected error for unregistered adapter")
	}
}
