package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/messaging"
)

const (
	registryListSubject = "zlaw.registry.list"
	listAgentsTimeout   = 5 * time.Second
)

// AgentRegistry abstracts the hub registry for query purposes.
type AgentRegistry interface {
	ListAgents(ctx context.Context) ([]hub.RegistryEntry, error)
	GetAgent(ctx context.Context, name string) (*hub.RegistryEntry, error)
}

// NATSAgentRegistry implements AgentRegistry over NATS request/reply.
type NATSAgentRegistry struct {
	Messenger messaging.Messenger
}

// ListAgents queries zlaw.registry.list and returns all registered agents.
func (r *NATSAgentRegistry) ListAgents(ctx context.Context) ([]hub.RegistryEntry, error) {
	if r.Messenger == nil {
		return nil, fmt.Errorf("list_agents: not connected to hub")
	}
	data, err := r.Messenger.Request(ctx, registryListSubject, nil, listAgentsTimeout)
	if err != nil {
		return nil, fmt.Errorf("list_agents: request to hub registry: %w", err)
	}
	var entries []hub.RegistryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("list_agents: parse registry response: %w", err)
	}
	return entries, nil
}

// GetAgent returns the registry entry for name, or nil if not found.
func (r *NATSAgentRegistry) GetAgent(ctx context.Context, name string) (*hub.RegistryEntry, error) {
	entries, err := r.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, nil
}

var _ AgentRegistry = (*NATSAgentRegistry)(nil)

// NewAgentRegistry returns a NATS-backed registry for the given messenger.
// When messenger is nil (standalone mode) the registry returns errors at call time.
func NewAgentRegistry(messenger messaging.Messenger) *NATSAgentRegistry {
	return &NATSAgentRegistry{Messenger: messenger}
}

// ListAgents is a read-only builtin tool that returns the list of live agents
// from the hub registry via NATS request/reply.
type ListAgents struct {
	Registry AgentRegistry
	AgentID  string // injected at registration time so the tool can mark "is_self"
}

var listAgentsSchema = []byte(`{
  "type": "object",
  "properties": {},
  "required": []
}`)

func (*ListAgents) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "list_agents",
		Description: "List all agents registered in the hub. Returns name, version, capabilities, roles, and connection status for each agent. The current agent is marked with is_self: true.",
		InputSchema: listAgentsSchema,
	}
}

// Execute queries the hub registry and returns the agent list as JSON.
// The current agent is marked with is_self: true.
func (t *ListAgents) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Registry == nil {
		return "", fmt.Errorf("list_agents: not connected to hub")
	}
	entries, err := t.Registry.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	// Mark the current agent.
	agentList := make([]map[string]any, len(entries))
	for i, e := range entries {
		m := map[string]any{
			"name":         e.Name,
			"version":      e.Version,
			"capabilities": e.Capabilities,
			"roles":        e.Roles,
			"status":       e.Status,
			"is_self":      e.Name == t.AgentID,
		}
		agentList[i] = m
	}
	out := map[string]any{"agents": agentList, "count": len(agentList)}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetAgent is a read-only builtin tool that returns a single agent's registry
// entry by name.
type GetAgent struct {
	Registry AgentRegistry
}

var getAgentSchema = []byte(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the agent to look up."
    }
  },
  "required": ["name"]
}`)

func (*GetAgent) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "get_agent",
		Description: "Get details for a specific agent by name. Returns version, capabilities, roles, and connection status.",
		InputSchema: getAgentSchema,
	}
}

type getAgentInput struct {
	Name string `json:"name"`
}

// Execute looks up name in the hub registry.
func (t *GetAgent) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Registry == nil {
		return "", fmt.Errorf("get_agent: not connected to hub")
	}
	var in getAgentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("get_agent: invalid input: %w", err)
	}
	if in.Name == "" {
		return "", fmt.Errorf("get_agent: name is required")
	}
	entry, err := t.Registry.GetAgent(ctx, in.Name)
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", fmt.Errorf("get_agent: agent %q not found in registry", in.Name)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
