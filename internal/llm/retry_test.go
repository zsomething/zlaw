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

// seqClient returns sequential errors/responses per call.
type seqClient struct {
	errs      []error
	responses []llm.Response
	calls     *int
}

func (s *seqClient) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	i := *s.calls
	*s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return llm.Response{}, s.errs[i]
	}
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return llm.Response{}, errors.New("seqClient: out of responses")
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

func TestRetryClient_RetriesOverloaded(t *testing.T) {
	inner := &failClient{
		failFor:  2,
		err:      llm.ErrOverloaded,
		response: llm.TextResponse("ok after overload"),
	}
	c := llm.NewRetryClient(inner, llm.RetryConfig{
		OverloadedMaxRetries: 3,
		OverloadedBaseDelay:  time.Millisecond,
	}, nil)

	resp, err := c.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.TextContent() != "ok after overload" {
		t.Fatalf("unexpected response: %v", resp)
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
}

func TestRetryClient_ExhaustsOverloadedRetries(t *testing.T) {
	inner := &failClient{failFor: 99, err: llm.ErrOverloaded}
	c := llm.NewRetryClient(inner, llm.RetryConfig{
		OverloadedMaxRetries: 2,
		OverloadedBaseDelay:  time.Millisecond,
	}, nil)

	_, err := c.Complete(context.Background(), llm.Request{})
	if err == nil {
		t.Fatal("expected error after exhausting overloaded retries")
	}
	if !errors.Is(err, llm.ErrOverloaded) {
		t.Fatalf("expected ErrOverloaded wrapped, got: %v", err)
	}
	// 1 initial + 2 retries = 3 calls
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
}

func TestRetryClient_OverloadedIndependentOfGeneralAttempts(t *testing.T) {
	// Overloaded retries should not consume the general attempt budget.
	// 2 overloaded failures then 2 general failures → 5 total calls,
	// succeeds on 5th (within general MaxAttempts=3 budget).
	calls := 0
	errs := []error{llm.ErrOverloaded, llm.ErrOverloaded, errors.New("transient"), errors.New("transient"), nil}
	responses := []llm.Response{{}, {}, {}, {}, llm.TextResponse("ok")}
	inner := &seqClient{errs: errs, responses: responses, calls: &calls}

	c := llm.NewRetryClient(inner, llm.RetryConfig{
		MaxAttempts:          3,
		BaseDelay:            time.Millisecond,
		OverloadedMaxRetries: 2,
		OverloadedBaseDelay:  time.Millisecond,
	}, nil)

	resp, err := c.Complete(context.Background(), llm.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Message.TextContent() != "ok" {
		t.Fatalf("unexpected response: %v", resp)
	}
	if calls != 5 {
		t.Fatalf("expected 5 calls, got %d", calls)
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
