// Package ctxkey defines typed context keys shared across packages to avoid
// collisions. Each key is an unexported type so only this package can produce
// values that satisfy it.
package ctxkey

import "context"

type key int

const (
	// SourceChannel is the full push address of the channel that submitted the
	// current turn, e.g. "telegram:123456789". Set by adapters before calling
	// session.Manager.Submit so tools can use it as a default delivery target.
	SourceChannel key = iota

	// SessionID is the current agent session identifier. Set by the agent loop
	// before executing tools so tools can propagate it to outbound requests.
	SessionID

	// TraceID is the distributed trace identifier for the current conversation.
	// It is generated once at session start and propagated across all agent
	// hops, delegation envelopes, and tool calls for distributed tracing.
	TraceID
)

// SessionIDFrom returns the session ID from ctx, or "" if not set.
func SessionIDFrom(ctx context.Context) string {
	if v := ctx.Value(SessionID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithSessionID returns a context with the given session ID.
func WithSessionID(parent context.Context, sessionID string) context.Context {
	return context.WithValue(parent, SessionID, sessionID)
}

// WithSourceChannel returns a context with the source channel address set.
func WithSourceChannel(parent context.Context, channel string) context.Context {
	return context.WithValue(parent, SourceChannel, channel)
}

// SourceChannelOf returns the source channel from ctx, or "" if not set.
func SourceChannelOf(ctx context.Context) string {
	if v := ctx.Value(SourceChannel); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithTraceID returns a context with traceID set.
func WithTraceID(parent context.Context, traceID string) context.Context {
	return context.WithValue(parent, TraceID, traceID)
}

// TraceIDOf returns the trace ID from ctx, or "" if not set.
func TraceIDOf(ctx context.Context) string {
	if v := ctx.Value(TraceID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
