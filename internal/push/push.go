// Package push defines the Pusher interface and Registry for outbound message
// delivery to a known target address without waiting for an inbound message.
// This is the shared foundation for cronjob, polling, and deferred execution.
package push

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Pusher delivers a message to a known address within an adapter's namespace.
// address is the adapter-specific part of a target (e.g. the chat ID for Telegram).
type Pusher interface {
	Push(ctx context.Context, address string, message string) error
}

// Registry routes Push calls to the appropriate Pusher based on a target prefix.
// Target format: "<adapter>:<address>", e.g. "telegram:123456789".
type Registry struct {
	mu      sync.RWMutex
	pushers map[string]Pusher
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{pushers: make(map[string]Pusher)}
}

// Register adds (or replaces) a Pusher for the given prefix (e.g. "telegram", "cli").
func (r *Registry) Register(prefix string, p Pusher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushers[prefix] = p
}

// Push parses target as "<prefix>:<address>" and delegates to the registered
// Pusher for that prefix. Returns an error if the format is invalid or no
// Pusher is registered for the prefix.
func (r *Registry) Push(ctx context.Context, target string, message string) error {
	prefix, address, ok := strings.Cut(target, ":")
	if !ok {
		return fmt.Errorf("push: invalid target %q: expected <adapter>:<address>", target)
	}
	r.mu.RLock()
	p, found := r.pushers[prefix]
	r.mu.RUnlock()
	if !found {
		return fmt.Errorf("push: no pusher registered for adapter %q", prefix)
	}
	return p.Push(ctx, address, message)
}
