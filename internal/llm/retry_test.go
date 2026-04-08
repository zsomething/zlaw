package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chickenzord/zlaw/internal/llm"
)

// failClient returns an error for the first N calls, then succeeds.
type failClient struct {
	failFor  int
	calls    int
	err      error
	response llm.Response
}

func (f *failClient) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	f.calls++
	if f.calls <= f.failFor {
		return llm.Response{}, f.err
	}
	return f.response, nil
}

func TestRetryClient_SucceedsOnFirstAttempt(t *testing.T) {
	inner := &failClient{failFor: 0, response: llm.TextResponse("ok")}
	c := llm.NewRetryClient(inner, llm.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, nil)

	resp, err := c.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.TextContent() != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 call, got %d", inner.calls)
	}
}

func TestRetryClient_RetriesTransientError(t *testing.T) {
	inner := &failClient{
		failFor:  2,
		err:      errors.New("transient"),
		response: llm.TextResponse("ok after retry"),
	}
	c := llm.NewRetryClient(inner, llm.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, nil)

	resp, err := c.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.TextContent() != "ok after retry" {
		t.Fatalf("unexpected response: %v", resp)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
}

func TestRetryClient_ExhaustsAttempts(t *testing.T) {
	sentinel := errors.New("always fails")
	inner := &failClient{failFor: 99, err: sentinel}
	c := llm.NewRetryClient(inner, llm.RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}, nil)

	_, err := c.Complete(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error wrapped, got: %v", err)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
}

func TestRetryClient_ContextCancelledStopsRetry(t *testing.T) {
	inner := &failClient{failFor: 99, err: errors.New("transient")}
	c := llm.NewRetryClient(inner, llm.RetryConfig{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.Complete(ctx, llm.Request{})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestRetryClient_DefaultConfig(t *testing.T) {
	sentinel := errors.New("always fails")
	inner := &failClient{failFor: 99, err: sentinel}
	// Zero config → defaults: 3 attempts, 500ms base (we pass 1ms to keep test fast)
	c := llm.NewRetryClient(inner, llm.RetryConfig{BaseDelay: time.Millisecond}, nil)

	_, err := c.Complete(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 attempts by default, got %d", inner.calls)
	}
}
