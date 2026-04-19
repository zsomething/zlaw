package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/tools"
)

// Tools returns the list of all built-in tool definitions.
func Tools() []tools.Definition {
	return tools.Tools()
}

// ToolRequest is the envelope for a tool-call request sent from an
// agent to the hub's NATS inbox subject (zlaw.hub.inbox).
type ToolRequest struct {
	// Tool is the name of the tool to invoke.
	Tool string `json:"tool"`
	// Args holds the tool arguments as key-value pairs.
	Args map[string]any `json:"args"`
	// ReplyTo is the subject the hub should publish the ToolReply to.
	ReplyTo string `json:"reply_to"`
}

// ToolReply is the response envelope for a tool-call request.
type ToolReply struct {
	// Tool is echoed from the request.
	Tool string `json:"tool"`
	// OK is true when the tool executed without error.
	OK bool `json:"ok"`
	// Output is the tool's output; present when OK is true.
	Output string `json:"output,omitempty"`
	// Error is the error message; present when OK is false.
	Error string `json:"error,omitempty"`
}

// ToolHandler is the interface for hub-side tool execution.
// It is implemented by HubInbox to route tool calls to the appropriate
// handler methods.
type ToolHandler interface {
	// HandleTool routes a tool request to the correct handler.
	// It returns the tool output or an error.
	HandleTool(ctx context.Context, req ToolRequest) ToolReply
}

// HubInbox processes incoming tool-call requests from agents via
// the NATS zlaw.hub.inbox subject. It dispatches each request to the
// appropriate ToolHandler method and publishes the reply to the specified
// reply-to subject.
//
// Unlike the ManagementHandler (which uses the ControlSocket for CLI access),
// HubInbox is the NATS-facing handler for agent-initiated tool calls.
// Both may share the same underlying Supervisor and Registry.
type HubInbox struct {
	supervisor ToolSupervisor
	registry   ToolRegistry
	cfg        ToolHubConfig
	logger     *slog.Logger
}

// ToolSupervisor is the subset of Supervisor needed by HubInbox.
type ToolSupervisor interface {
	Status(name string) (AgentStatus, error)
	Stop(name string) error
	Restart(name string) error
}

// ToolRegistry is the subset of Registry needed by HubInbox.
type ToolRegistry interface {
	List() []RegistryEntry
	Get(name string) (RegistryEntry, bool)
}

// ToolHubConfig provides static hub configuration for tool handlers.
type ToolHubConfig interface {
	HubName() string
}

// hubConfigAdapter adapts HubConfig to ToolHubConfig.
type hubConfigAdapter struct{ cfg config.HubConfig }

func (a hubConfigAdapter) HubName() string { return a.cfg.Hub.Name }

var _ ToolHubConfig = hubConfigAdapter{}

// NewHubInbox creates a HubInbox with the given dependencies.
func NewHubInbox(
	supervisor ToolSupervisor,
	registry ToolRegistry,
	cfg ToolHubConfig,
	logger *slog.Logger,
) *HubInbox {
	if logger == nil {
		logger = slog.Default()
	}
	return &HubInbox{
		supervisor: supervisor,
		registry:   registry,
		cfg:        cfg,
		logger:     logger,
	}
}

// HandleTool routes a tool request to the correct handler.
func (h *HubInbox) HandleTool(ctx context.Context, req ToolRequest) ToolReply {
	return h.HandleToolRequest(ctx, req)
}

// HandleToolRequest dispatches a ToolRequest to the correct handler and
// returns the reply. It is the pure-logic version used by both the NATS
// tool inbox and the control socket.
func (h *HubInbox) HandleToolRequest(ctx context.Context, req ToolRequest) ToolReply {
	switch strings.ToLower(req.Tool) {
	case "hub_status":
		return h.hubStatus()
	case "agent_status":
		return h.agentStatus(ctx, req.Args)
	case "agent_stop":
		return h.agentStop(ctx, req.Args)
	case "agent_restart":
		return h.agentRestart(ctx, req.Args)
	case "agent_list":
		return h.agentList()
	case "agent_get":
		return h.agentGet(ctx, req.Args)
	default:
		return ToolReply{
			Tool:  req.Tool,
			OK:    false,
			Error: "unknown hub tool: " + req.Tool,
		}
	}
}

// hubStatus returns static hub information.
func (h *HubInbox) hubStatus() ToolReply {
	var jsEnabled bool
	if h.cfg != nil {
		jsEnabled = true // actual value comes from NATSResult; approximate here
	}
	_ = jsEnabled
	return ToolReply{
		Tool: "hub_status",
		OK:   true,
		Output: marshalJSON(map[string]any{
			"name":        h.cfg.HubName(),
			"jetstream":   jsEnabled,
			"tool_routed": true,
		}),
	}
}

// agentStatus returns the status of an agent.
func (h *HubInbox) agentStatus(ctx context.Context, args map[string]any) ToolReply {
	id := stringArg(args, "id")
	if id == "" {
		return errorReply("agent_status", "param 'id' is required")
	}
	status, err := h.supervisor.Status(id)
	if err != nil {
		return errorReply("agent_status", err.Error())
	}
	return ToolReply{
		Tool:   "agent_status",
		OK:     true,
		Output: marshalJSON(status),
	}
}

// agentStop stops an agent.
func (h *HubInbox) agentStop(ctx context.Context, args map[string]any) ToolReply {
	id := stringArg(args, "id")
	if id == "" {
		return errorReply("agent_stop", "param 'id' is required")
	}
	if err := h.supervisor.Stop(id); err != nil {
		return errorReply("agent_stop", err.Error())
	}
	return ToolReply{
		Tool:   "agent_stop",
		OK:     true,
		Output: id + " stopped",
	}
}

// agentRestart restarts an agent.
func (h *HubInbox) agentRestart(ctx context.Context, args map[string]any) ToolReply {
	id := stringArg(args, "id")
	if id == "" {
		return errorReply("agent_restart", "param 'id' is required")
	}
	if err := h.supervisor.Restart(id); err != nil {
		return errorReply("agent_restart", err.Error())
	}
	return ToolReply{
		Tool:   "agent_restart",
		OK:     true,
		Output: id + " restarted",
	}
}

// agentList returns all registered agents.
func (h *HubInbox) agentList() ToolReply {
	entries := h.registry.List()
	return ToolReply{
		Tool:   "agent_list",
		OK:     true,
		Output: marshalJSON(entries),
	}
}

// agentGet returns the registry entry for an agent.
func (h *HubInbox) agentGet(ctx context.Context, args map[string]any) ToolReply {
	id := stringArg(args, "id")
	if id == "" {
		return errorReply("agent_get", "param 'id' is required")
	}
	entry, ok := h.registry.Get(id)
	if !ok {
		return errorReply("agent_get", "agent not found: "+id)
	}
	return ToolReply{
		Tool:   "agent_get",
		OK:     true,
		Output: marshalJSON(entry),
	}
}

// errorReply creates an error ToolReply for the given tool name.
func errorReply(tool, msg string) ToolReply {
	return ToolReply{Tool: tool, OK: false, Error: msg}
}

// stringArg extracts a string value from args by key.
func stringArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// marshalJSON marshals v as JSON, panicking on error (should not happen).
func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"marshal error"}`
	}
	return string(b)
}

// ProcessInboxMessage parses a ToolRequest from the raw payload,
// dispatches it via HandleTool, and returns the result.
// The caller is responsible for publishing the reply to ReplyTo.
// It returns false if the payload is not a valid ToolRequest.
func (h *HubInbox) ProcessInboxMessage(ctx context.Context, data []byte) (ToolReply, bool) {
	var req ToolRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return ToolReply{}, false
	}
	if req.Tool == "" || req.ReplyTo == "" {
		return ToolReply{}, false
	}
	return h.HandleToolRequest(ctx, req), true
}

// compile-time interface check
var _ ToolHandler = (*HubInbox)(nil)
