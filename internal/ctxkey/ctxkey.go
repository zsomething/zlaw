// Package ctxkey defines typed context keys shared across packages to avoid
// collisions. Each key is an unexported type so only this package can produce
// values that satisfy it.
package ctxkey

type key int

const (
	// SourceChannel is the full push address of the channel that submitted the
	// current turn, e.g. "telegram:123456789". Set by adapters before calling
	// session.Manager.Submit so tools can use it as a default delivery target.
	SourceChannel key = iota

	// SessionID is the current agent session identifier. Set by the agent loop
	// before executing tools so tools can propagate it to outbound requests.
	SessionID
)
