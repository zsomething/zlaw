package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/openai/openai-go"
)

// RetryConfig controls the retry behaviour of RetryClient.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (initial + retries) for general
	// and rate-limit errors. Zero or negative values are treated as 3.
	MaxAttempts int
	// BaseDelay is the initial backoff duration for general and rate-limit errors.
	// Zero uses 500ms.
	BaseDelay time.Duration

	// OverloadedMaxRetries is the number of retries for ErrOverloaded (HTTP 529).
	// Zero uses 2.
	OverloadedMaxRetries int
	// OverloadedBaseDelay is the starting delay between overloaded retries.
	// Zero uses 30s. Delay grows exponentially: BaseDelay * 2^(retry-1).
	OverloadedBaseDelay time.Duration
}

func (c RetryConfig) maxAttempts() int {
	if c.MaxAttempts <= 0 {
		return 3
	}
	return c.MaxAttempts
}

func (c RetryConfig) baseDelay() time.Duration {
	if c.BaseDelay == 0 {
		return 500 * time.Millisecond
	}
	return c.BaseDelay
}

func (c RetryConfig) overloadedMaxRetries() int {
	if c.OverloadedMaxRetries <= 0 {
		return 2
	}
	return c.OverloadedMaxRetries
}

func (c RetryConfig) overloadedBaseDelay() time.Duration {
	if c.OverloadedBaseDelay == 0 {
		return 30 * time.Second
	}
	return c.OverloadedBaseDelay
}

// RetryClient wraps a Client and retries transient errors with exponential
// backoff and jitter.
//
// Retryable conditions:
//   - ErrRateLimit (HTTP 429): exponential backoff; honours Retry-After when present
//   - ErrOverloaded (HTTP 529): separate attempt counter, longer exponential backoff
//   - Any other error: exponential backoff up to MaxAttempts
//
// Overloaded retries are tracked independently of general retries so that a
// burst of 529s does not exhaust the general MaxAttempts budget.
//
// Context cancellation and deadline exceeded are never retried.
type RetryClient struct {
	inner  Client
	cfg    RetryConfig
	logger *slog.Logger
}

// NewRetryClient wraps inner with retry logic using cfg.
// If logger is nil, slog.Default() is used.
func NewRetryClient(inner Client, cfg RetryConfig, logger *slog.Logger) *RetryClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetryClient{inner: inner, cfg: cfg, logger: logger}
}

// Complete calls the wrapped client, retrying on transient failures.
func (r *RetryClient) Complete(ctx context.Context, req Request) (Response, error) {
	generalAttempt := 0
	overloadedRetry := 0

	for {
		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return Response{}, err
		}

		if errors.Is(err, ErrOverloaded) {
			overloadedRetry++
			if overloadedRetry > r.cfg.overloadedMaxRetries() {
				return Response{}, fmt.Errorf("llm: overloaded, %d retries exhausted: %w", r.cfg.overloadedMaxRetries(), err)
			}
			delay := r.overloadedDelay(overloadedRetry)
			r.logger.Warn("llm: overloaded, retrying",
				"retry", overloadedRetry,
				"max_retries", r.cfg.overloadedMaxRetries(),
				"delay", delay,
				"error", err)
			if err := r.sleep(ctx, delay); err != nil {
				return Response{}, err
			}
			continue
		}

		generalAttempt++
		if generalAttempt >= r.cfg.maxAttempts() {
			return Response{}, fmt.Errorf("llm: all %d attempts failed: %w", r.cfg.maxAttempts(), err)
		}
		delay := r.backoffDelay(generalAttempt, err)
		r.logger.Warn("llm: retrying after error",
			"attempt", generalAttempt,
			"max_attempts", r.cfg.maxAttempts(),
			"delay", delay,
			"error", err)
		if err := r.sleep(ctx, delay); err != nil {
			return Response{}, err
		}
	}
}

// CompleteStream delegates to the inner client's CompleteStream if it supports
// streaming, otherwise falls back to Complete without streaming.
// Streaming responses are not retried once the stream begins.
func (r *RetryClient) CompleteStream(ctx context.Context, req Request, handler StreamHandler) (Response, error) {
	if sc, ok := r.inner.(StreamingClient); ok {
		return sc.CompleteStream(ctx, req, handler)
	}
	return r.Complete(ctx, req)
}

func (r *RetryClient) sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("llm: retry cancelled: %w", ctx.Err())
	case <-time.After(d):
		return nil
	}
}

// backoffDelay returns the delay before the next general/rate-limit attempt.
// For rate-limit errors it prefers the Retry-After header when present.
func (r *RetryClient) backoffDelay(attempt int, err error) time.Duration {
	if errors.Is(err, ErrRateLimit) {
		if d := retryAfterDelay(err); d > 0 {
			return d
		}
	}
	return jitteredExp(r.cfg.baseDelay(), attempt)
}

// overloadedDelay returns the delay before the next overloaded retry.
func (r *RetryClient) overloadedDelay(retry int) time.Duration {
	return jitteredExp(r.cfg.overloadedBaseDelay(), retry)
}

// jitteredExp returns BaseDelay * 2^(n-1) with ±25% jitter.
func jitteredExp(base time.Duration, n int) time.Duration {
	exp := base * (1 << (n - 1))
	jitter := time.Duration(rand.Int63n(int64(exp) / 2)) // 0..50% of exp
	if rand.Intn(2) == 0 {
		return exp + jitter/2
	}
	return exp - jitter/2
}

// retryAfterDelay extracts the Retry-After value from an openai API error if
// present. Returns 0 when no usable value is found.
func retryAfterDelay(err error) time.Duration {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || apiErr.Response == nil {
		return 0
	}

	ra := apiErr.Response.Header.Get("Retry-After")
	if ra == "" {
		return 0
	}

	// Retry-After may be a delta-seconds integer or an HTTP-date.
	if secs, parseErr := strconv.Atoi(ra); parseErr == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, parseErr := http.ParseTime(ra); parseErr == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
