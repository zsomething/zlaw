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
	// MaxAttempts is the total number of attempts (initial + retries).
	// Zero or negative values are treated as 3.
	MaxAttempts int
	// BaseDelay is the initial backoff duration before the first retry.
	// Zero uses 500ms.
	BaseDelay time.Duration
}

// RetryClient wraps a Client and retries transient errors with exponential
// backoff and jitter.
//
// Retryable conditions:
//   - ErrRateLimit (HTTP 429): backs off, honours Retry-After when present
//   - Any other error: retried with exponential backoff up to MaxAttempts
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
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = 500 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RetryClient{inner: inner, cfg: cfg, logger: logger}
}

// Complete calls the wrapped client, retrying on transient failures.
func (r *RetryClient) Complete(ctx context.Context, req Request) (Response, error) {
	var lastErr error
	for attempt := 1; attempt <= r.cfg.MaxAttempts; attempt++ {
		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}

		// Never retry if the context is done.
		if ctx.Err() != nil {
			return Response{}, err
		}

		lastErr = err

		if attempt == r.cfg.MaxAttempts {
			break
		}

		delay := r.backoffDelay(attempt, err)
		r.logger.Warn("llm: retrying after error",
			"attempt", attempt,
			"max_attempts", r.cfg.MaxAttempts,
			"delay", delay,
			"error", err)

		select {
		case <-ctx.Done():
			return Response{}, fmt.Errorf("llm: retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}
	return Response{}, fmt.Errorf("llm: all %d attempts failed: %w", r.cfg.MaxAttempts, lastErr)
}

// backoffDelay returns the delay before the next attempt.
// For rate-limit errors it prefers the Retry-After header when present.
func (r *RetryClient) backoffDelay(attempt int, err error) time.Duration {
	if errors.Is(err, ErrRateLimit) {
		if d := retryAfterDelay(err); d > 0 {
			return d
		}
	}
	// Exponential backoff with ±25% jitter.
	exp := r.cfg.BaseDelay * (1 << (attempt - 1))
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
