package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/zsomething/zlaw/internal/messaging"
)

// registrationMsg mirrors the payload agents publish to zlaw.registry.
type registrationMsg struct {
	Name string `json:"name"`
}

// RegistryCache subscribes to zlaw.registry and maintains a local set of known
// agent IDs. It is used by the agent_delegate tool to validate target agents
// before attempting to send a TaskEnvelope.
type RegistryCache struct {
	mu     sync.RWMutex
	agents map[string]bool
	logger *slog.Logger
}

// NewRegistryCache returns an uninitialised RegistryCache. Call Start to begin
// listening for registrations.
func NewRegistryCache(logger *slog.Logger) *RegistryCache {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegistryCache{
		agents: make(map[string]bool),
		logger: logger,
	}
}

// Start subscribes to the zlaw.registry subject and begins tracking agent
// registrations. It returns when ctx is cancelled.
func (rc *RegistryCache) Start(ctx context.Context, m messaging.Messenger) error {
	sub, err := m.Subscribe(ctx, registrySubject, func(data []byte) {
		var msg registrationMsg
		if err := json.Unmarshal(data, &msg); err != nil || msg.Name == "" {
			return
		}
		rc.mu.Lock()
		rc.agents[msg.Name] = true
		rc.mu.Unlock()
		rc.logger.Debug("registry cache: agent seen", "agent", msg.Name)
	})
	if err != nil {
		return err
	}

	<-ctx.Done()
	sub.Unsubscribe() //nolint:errcheck
	return nil
}

// IsRegistered reports whether an agent with the given ID has been seen in the
// registry within the current process lifetime.
func (rc *RegistryCache) IsRegistered(id string) bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.agents[id]
}
