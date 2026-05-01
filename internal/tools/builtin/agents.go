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
	hubInboxSubject     = "zlaw.hub.inbox"
	registryListSubject = "zlaw.registry.list"
	hubToolTimeout      = 10 * time.Second
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
		return nil, fmt.Errorf("agent_list: not connected to hub")
	}
	data, err := r.Messenger.Request(ctx, registryListSubject, nil, hubToolTimeout)
	if err != nil {
		return nil, fmt.Errorf("agent_list: request to hub registry: %w", err)
	}
	var entries []hub.RegistryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("agent_list: parse registry response: %w", err)
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
		if entries[i].ID == name {
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

// HubMessenger is the interface for sending tool requests to the hub inbox.
type HubMessenger interface {
	Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error)
}

// HubMessengerFunc adapts a function to HubMessenger.
type HubMessengerFunc func(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error)

func (f HubMessengerFunc) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	return f(ctx, subject, payload, timeout)
}

// AgentList is a read-only builtin tool that returns the list of live agents
// from the hub registry via NATS request/reply.
type AgentList struct {
	Registry AgentRegistry
	AgentID  string // injected at registration time so the tool can mark "is_self"
}

var agentListSchema = []byte(`{
  "type": "object",
  "properties": {},
  "required": []
}`)

func (*AgentList) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "agent_list",
		Description: "List all agents registered in the hub. Returns name, version, capabilities, roles, and connection status for each agent. The current agent is marked with is_self: true.",
		InputSchema: agentListSchema,
	}
}

// Execute queries the hub registry and returns the agent list as JSON.
// The current agent is marked with is_self: true.
func (t *AgentList) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Registry == nil {
		return "", fmt.Errorf("agent_list: not connected to hub")
	}
	entries, err := t.Registry.ListAgents(ctx)
	if err != nil {
		return "", err
	}
	// Mark the current agent.
	agentListOut := make([]map[string]any, len(entries))
	for i, e := range entries {
		m := map[string]any{
			"id":           e.ID,
			"version":      e.Version,
			"capabilities": e.Capabilities,
			"roles":        e.Roles,
			"status":       e.Status,
			"is_self":      e.ID == t.AgentID,
		}
		agentListOut[i] = m
	}
	out := map[string]any{"agents": agentListOut, "count": len(agentListOut)}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AgentGet is a read-only builtin tool that returns a single agent's registry
// entry by name.
type AgentGet struct {
	Registry AgentRegistry
}

var agentGetSchema = []byte(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The name of the agent to look up."
    }
  },
  "required": ["name"]
}`)

func (*AgentGet) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "agent_get",
		Description: "Get details for a specific agent by name. Returns version, capabilities, roles, and connection status.",
		InputSchema: agentGetSchema,
	}
}

type agentGetInput struct {
	ID string `json:"id"`
}

// Execute looks up id in the hub registry.
func (t *AgentGet) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Registry == nil {
		return "", fmt.Errorf("agent_get: not connected to hub")
	}
	var in agentGetInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("agent_get: invalid input: %w", err)
	}
	if in.ID == "" {
		return "", fmt.Errorf("agent_get: id is required")
	}
	entry, err := t.Registry.GetAgent(ctx, in.ID)
	if err != nil {
		return "", err
	}
	if entry == nil {
		return "", fmt.Errorf("agent_get: agent %q not found in registry", in.ID)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AgentStop stops a running agent by ID.
type AgentStop struct {
	SelfID    string
	Messenger HubMessenger
}

var agentStopSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "The ID of the agent to stop."
    }
  },
  "required": ["id"]
}`)

func (*AgentStop) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "agent_stop",
		Description: "Stop a running agent by ID.",
		InputSchema: agentStopSchema,
	}
}

type agentStopInput struct {
	ID string `json:"id"`
}

func (t *AgentStop) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Messenger == nil {
		return "", fmt.Errorf("agent_stop: not connected to hub")
	}
	var in agentStopInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("agent_stop: invalid input: %w", err)
	}
	if in.ID == "" {
		return "", fmt.Errorf("agent_stop: id is required")
	}
	// Self-protection: cannot stop self
	if in.ID == t.SelfID {
		return "", fmt.Errorf("agent_stop: cannot stop self. Operation refused")
	}

	// Forward to hub inbox
	payload, _ := json.Marshal(map[string]any{
		"tool":     "agent_stop",
		"args":     map[string]any{"id": in.ID},
		"reply_to": "_INBOX.>",
	})
	data, err := t.Messenger.Request(ctx, hubInboxSubject, payload, hubToolTimeout)
	if err != nil {
		return "", fmt.Errorf("agent_stop: request to hub: %w", err)
	}

	var reply struct {
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return "", fmt.Errorf("agent_stop: parse hub reply: %w", err)
	}
	if !reply.OK {
		return "", fmt.Errorf("agent_stop: %s", reply.Error)
	}
	return reply.Output, nil
}

// AgentRestart restarts a stopped or running agent by ID.
type AgentRestart struct {
	SelfID    string
	Messenger HubMessenger
}

var agentRestartSchema = []byte(`{
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "The ID of the agent to restart."
    }
  },
  "required": ["id"]
}`)

func (*AgentRestart) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Name:        "agent_restart",
		Description: "Restart an agent by ID. The agent will be stopped and re-spawned.",
		InputSchema: agentRestartSchema,
	}
}

type agentRestartInput struct {
	ID string `json:"id"`
}

func (t *AgentRestart) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if t.Messenger == nil {
		return "", fmt.Errorf("agent_restart: not connected to hub")
	}
	var in agentRestartInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("agent_restart: invalid input: %w", err)
	}
	if in.ID == "" {
		return "", fmt.Errorf("agent_restart: id is required")
	}
	// Self-protection: cannot restart self
	if in.ID == t.SelfID {
		return "", fmt.Errorf("agent_restart: cannot restart self. Operation refused")
	}

	// Forward to hub inbox
	payload, _ := json.Marshal(map[string]any{
		"tool":     "agent_restart",
		"args":     map[string]any{"id": in.ID},
		"reply_to": "_INBOX.>",
	})
	data, err := t.Messenger.Request(ctx, hubInboxSubject, payload, hubToolTimeout)
	if err != nil {
		return "", fmt.Errorf("agent_restart: request to hub: %w", err)
	}

	var reply struct {
		OK     bool   `json:"ok"`
		Output string `json:"output,omitempty"`
		Error  string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return "", fmt.Errorf("agent_restart: parse hub reply: %w", err)
	}
	if !reply.OK {
		return "", fmt.Errorf("agent_restart: %s", reply.Error)
	}
	return reply.Output, nil
}
