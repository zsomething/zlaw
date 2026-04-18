package ctxkey_test

import (
	"context"
	"testing"

	"github.com/zsomething/zlaw/internal/ctxkey"
)

func TestSessionIDFrom_NotSet(t *testing.T) {
	ctx := context.Background()
	got := ctxkey.SessionIDFrom(ctx)
	if got != "" {
		t.Errorf("SessionIDFrom(empty ctx) = %q, want %q", got, "")
	}
}

func TestSessionIDFrom_Set(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithSessionID(ctx, "test-session-123")
	got := ctxkey.SessionIDFrom(ctx)
	if got != "test-session-123" {
		t.Errorf("SessionIDFrom(ctx) = %q, want %q", got, "test-session-123")
	}
}

func TestSessionIDFrom_WrongType(t *testing.T) {
	ctx := context.Background()
	// Put a non-string value at SessionID key
	ctx = context.WithValue(ctx, ctxkey.SessionID, 42)
	got := ctxkey.SessionIDFrom(ctx)
	if got != "" {
		t.Errorf("SessionIDFrom(ctx with wrong type) = %q, want %q", got, "")
	}
}

func TestWithSessionID(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithSessionID(ctx, "my-session")
	got := ctxkey.SessionIDFrom(ctx)
	if got != "my-session" {
		t.Errorf("WithSessionID -> SessionIDFrom = %q, want %q", got, "my-session")
	}
}

func TestSourceChannelOf_NotSet(t *testing.T) {
	ctx := context.Background()
	got := ctxkey.SourceChannelOf(ctx)
	if got != "" {
		t.Errorf("SourceChannelOf(empty ctx) = %q, want %q", got, "")
	}
}

func TestSourceChannelOf_Set(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithSourceChannel(ctx, "telegram:123456789")
	got := ctxkey.SourceChannelOf(ctx)
	if got != "telegram:123456789" {
		t.Errorf("SourceChannelOf(ctx) = %q, want %q", got, "telegram:123456789")
	}
}

func TestSourceChannelOf_WrongType(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxkey.SourceChannel, 999)
	got := ctxkey.SourceChannelOf(ctx)
	if got != "" {
		t.Errorf("SourceChannelOf(ctx with wrong type) = %q, want %q", got, "")
	}
}

func TestWithSourceChannel(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithSourceChannel(ctx, "cli:local")
	got := ctxkey.SourceChannelOf(ctx)
	if got != "cli:local" {
		t.Errorf("WithSourceChannel -> SourceChannelOf = %q, want %q", got, "cli:local")
	}
}

func TestTraceIDOf_NotSet(t *testing.T) {
	ctx := context.Background()
	got := ctxkey.TraceIDOf(ctx)
	if got != "" {
		t.Errorf("TraceIDOf(empty ctx) = %q, want %q", got, "")
	}
}

func TestTraceIDOf_Set(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithTraceID(ctx, "trace-abc-123")
	got := ctxkey.TraceIDOf(ctx)
	if got != "trace-abc-123" {
		t.Errorf("TraceIDOf(ctx) = %q, want %q", got, "trace-abc-123")
	}
}

func TestTraceIDOf_WrongType(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxkey.TraceID, nil)
	got := ctxkey.TraceIDOf(ctx)
	if got != "" {
		t.Errorf("TraceIDOf(ctx with nil) = %q, want %q", got, "")
	}
}

func TestWithTraceID(t *testing.T) {
	ctx := context.Background()
	ctx = ctxkey.WithTraceID(ctx, "trace-xyz")
	got := ctxkey.TraceIDOf(ctx)
	if got != "trace-xyz" {
		t.Errorf("WithTraceID -> TraceIDOf = %q, want %q", got, "trace-xyz")
	}
}

func TestContextKeyTypesAreDistinct(t *testing.T) {
	// Ensure the key types are truly distinct by checking they don't collide
	ctx := context.Background()
	ctx = ctxkey.WithSessionID(ctx, "session")
	ctx = ctxkey.WithSourceChannel(ctx, "channel")
	ctx = ctxkey.WithTraceID(ctx, "trace")

	if got := ctxkey.SessionIDFrom(ctx); got != "session" {
		t.Errorf("SessionIDFrom after all set = %q, want %q", got, "session")
	}
	if got := ctxkey.SourceChannelOf(ctx); got != "channel" {
		t.Errorf("SourceChannelOf after all set = %q, want %q", got, "channel")
	}
	if got := ctxkey.TraceIDOf(ctx); got != "trace" {
		t.Errorf("TraceIDOf after all set = %q, want %q", got, "trace")
	}
}

func TestAllKeysWorkTogether(t *testing.T) {
	ctx := context.Background()

	// Can set multiple values independently
	ctx = ctxkey.WithSessionID(ctx, "s1")
	ctx = ctxkey.WithSourceChannel(ctx, "telegram:1")
	ctx = ctxkey.WithTraceID(ctx, "t1")

	// Can overwrite
	ctx = ctxkey.WithSessionID(ctx, "s2")

	if got := ctxkey.SessionIDFrom(ctx); got != "s2" {
		t.Errorf("SessionIDFrom after overwrite = %q, want %q", got, "s2")
	}
	// Others unchanged
	if got := ctxkey.SourceChannelOf(ctx); got != "telegram:1" {
		t.Errorf("SourceChannelOf after session overwrite = %q, want %q", got, "telegram:1")
	}
	if got := ctxkey.TraceIDOf(ctx); got != "t1" {
		t.Errorf("TraceIDOf after session overwrite = %q, want %q", got, "t1")
	}
}

func TestParentContextUnaffected(t *testing.T) {
	parent := context.Background()
	ctx := ctxkey.WithSessionID(parent, "child-session")

	// Parent should not have the value
	if got := ctxkey.SessionIDFrom(parent); got != "" {
		t.Errorf("Parent context unexpectedly has session ID %q", got)
	}

	// Child should have the value
	if got := ctxkey.SessionIDFrom(ctx); got != "child-session" {
		t.Errorf("Child context missing session ID %q, got %q", "child-session", got)
	}
}
