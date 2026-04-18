package web

import (
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

// StateFuncs is a func-set implementation of [State].
// Use this when the hub's components are not yet abstracted behind an interface.
type StateFuncs struct {
	HubConfigFn    func() config.HubConfig
	NATSAddrFn     func() string
	AgentsFn       func() []AgentInfo
	AuditEntriesFn func(limit int, eventType string) ([]hub.AuditEntry, error)
}

func (s StateFuncs) HubConfig() config.HubConfig { return s.HubConfigFn() }
func (s StateFuncs) NATSAddr() string            { return s.NATSAddrFn() }
func (s StateFuncs) Agents() []AgentInfo         { return s.AgentsFn() }
func (s StateFuncs) AuditEntries(limit int, eventType string) ([]hub.AuditEntry, error) {
	return s.AuditEntriesFn(limit, eventType)
}
