package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	// registrySubject is the NATS subject agents publish registration/heartbeat messages to.
	registrySubject = "zlaw.registry"

	// heartbeatInterval is how often agents are expected to publish heartbeats.
	heartbeatInterval = 30 * time.Second

	// maxMissedHeartbeats is how many heartbeat intervals may elapse before
	// an agent is marked disconnected.
	maxMissedHeartbeats = 2
)

// AgentConnStatus describes the connectivity state of a registered agent.
type AgentConnStatus string

const (
	// AgentConnected means the agent has sent a recent heartbeat.
	AgentConnected AgentConnStatus = "connected"
	// AgentDisconnected means the agent missed too many heartbeats.
	AgentDisconnected AgentConnStatus = "disconnected"
)

// RegistryEntry holds the live state of a single registered agent.
type RegistryEntry struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Capabilities  []string        `json:"capabilities"`
	Roles         []string        `json:"roles"`
	Status        AgentConnStatus `json:"status"`
	LastHeartbeat time.Time       `json:"last_heartbeat"`
}

// registrationMsg is the payload agents publish to zlaw.registry.
type registrationMsg struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Roles        []string `json:"roles"`
}

// Registry subscribes to zlaw.registry and maintains the live state of all
// connected agents. It marks agents as disconnected after maxMissedHeartbeats
// consecutive missed heartbeat intervals.
type Registry struct {
	logger *slog.Logger

	mu      sync.RWMutex
	entries map[string]*RegistryEntry
}

// NewRegistry creates an uninitialised Registry. Call Start to begin listening.
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		logger:  logger,
		entries: make(map[string]*RegistryEntry),
	}
}

// Start subscribes to NATS and begins the heartbeat monitor. It returns when
// ctx is cancelled.
func (r *Registry) Start(ctx context.Context, nc *nats.Conn) error {
	sub, err := nc.Subscribe(registrySubject, func(msg *nats.Msg) {
		r.handleRegistration(msg.Data)
	})
	if err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				sub.Unsubscribe() //nolint:errcheck
				return
			case <-ticker.C:
				r.checkHeartbeats()
			}
		}
	}()

	return nil
}

// List returns a snapshot of all registry entries.
func (r *Registry) List() []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, *e)
	}
	return out
}

// Get returns the registry entry for the named agent, and whether it exists.
func (r *Registry) Get(name string) (RegistryEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return RegistryEntry{}, false
	}
	return *e, true
}

// Deregister removes the named agent from the registry.
func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, name)
}

// handleRegistration parses an incoming registration/heartbeat message and
// upserts the corresponding registry entry.
func (r *Registry) handleRegistration(data []byte) {
	var msg registrationMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		r.logger.Warn("registry: invalid registration message", "err", err)
		return
	}
	if msg.Name == "" {
		r.logger.Warn("registry: registration message missing name")
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[msg.Name]
	if !exists {
		entry = &RegistryEntry{Name: msg.Name}
		r.entries[msg.Name] = entry
		r.logger.Info("registry: agent registered", "agent", msg.Name, "version", msg.Version)
	} else {
		r.logger.Debug("registry: heartbeat received", "agent", msg.Name)
	}

	entry.Version = msg.Version
	entry.Capabilities = msg.Capabilities
	entry.Status = AgentConnected
	entry.LastHeartbeat = time.Now()
}

// HandleQuery responds to a registry list request with the full agent list.
// It is idempotent — the caller receives a point-in-time snapshot.
func (r *Registry) HandleQuery(_ context.Context, _ []byte) ([]byte, error) {
	r.mu.RLock()
	entries := r.List()
	r.mu.RUnlock()
	return json.Marshal(entries)
}
func (r *Registry) checkHeartbeats() {
	deadline := time.Now().Add(-time.Duration(maxMissedHeartbeats) * heartbeatInterval)

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.entries {
		if entry.Status == AgentConnected && entry.LastHeartbeat.Before(deadline) {
			entry.Status = AgentDisconnected
			r.logger.Warn("registry: agent disconnected (missed heartbeats)",
				"agent", entry.Name,
				"last_heartbeat", entry.LastHeartbeat,
			)
		}
	}
}
