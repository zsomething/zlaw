package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/zsomething/zlaw/internal/config"
)

// HubToolDefinition describes a hub-level built-in tool.
type HubToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  []HubToolParam `json:"parameters"`
}

// HubToolParam describes a single parameter for a hub tool.
type HubToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// GlobalTools returns the list of all built-in tools.
// Some tools require hub connection (agent_*, hub_status) while others
// execute locally on each agent (read, write, bash, etc.).
func GlobalTools() []HubToolDefinition {
	return []HubToolDefinition{
		// File operations (local)
		{Name: "read", Description: "Read the contents of a file.", Parameters: []HubToolParam{
			{Name: "path", Type: "string", Description: "Path to file to read", Required: true},
			{Name: "offset", Type: "number", Description: "Line offset to start reading", Required: false},
			{Name: "limit", Type: "number", Description: "Max lines to read", Required: false},
		}},
		{Name: "write", Description: "Write content to a file. Creates parent directories if needed.", Parameters: []HubToolParam{
			{Name: "path", Type: "string", Description: "Path to file to write", Required: true},
			{Name: "content", Type: "string", Description: "Content to write", Required: true},
		}},
		{Name: "edit", Description: "Replace exact string in a file. Fails if string not found or ambiguous.", Parameters: []HubToolParam{
			{Name: "path", Type: "string", Description: "Path to file", Required: true},
			{Name: "old_string", Type: "string", Description: "String to replace", Required: true},
			{Name: "new_string", Type: "string", Description: "Replacement string", Required: true},
		}},
		{Name: "glob", Description: "Find files matching a glob pattern.", Parameters: []HubToolParam{
			{Name: "pattern", Type: "string", Description: "Glob pattern (e.g., **/*.go)", Required: true},
		}},
		{Name: "grep", Description: "Search for text within files using regex.", Parameters: []HubToolParam{
			{Name: "pattern", Type: "string", Description: "Regex pattern", Required: true},
			{Name: "path", Type: "string", Description: "Directory to search", Required: false},
			{Name: "file_pattern", Type: "string", Description: "File glob pattern", Required: false},
		}},

		// System (local)
		{Name: "bash", Description: "Execute a shell command.", Parameters: []HubToolParam{
			{Name: "command", Type: "string", Description: "Shell command to execute", Required: true},
			{Name: "cwd", Type: "string", Description: "Working directory", Required: false},
			{Name: "timeout", Type: "number", Description: "Timeout in seconds", Required: false},
		}},

		// Web (local)
		{Name: "web_fetch", Description: "Fetch content from a URL.", Parameters: []HubToolParam{
			{Name: "url", Type: "string", Description: "URL to fetch", Required: true},
			{Name: "prompt", Type: "string", Description: "Extract specific info from page", Required: false},
		}},
		{Name: "web_search", Description: "Search the web.", Parameters: []HubToolParam{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "top_n", Type: "number", Description: "Number of results", Required: false},
		}},
		{Name: "http_request", Description: "Make an HTTP request.", Parameters: []HubToolParam{
			{Name: "method", Type: "string", Description: "HTTP method", Required: true},
			{Name: "url", Type: "string", Description: "Request URL", Required: true},
			{Name: "headers", Type: "object", Description: "Request headers", Required: false},
			{Name: "body", Type: "string", Description: "Request body", Required: false},
		}},

		// Memory (local)
		{Name: "memory_save", Description: "Store information in persistent memory.", Parameters: []HubToolParam{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
			{Name: "value", Type: "string", Description: "Value to store", Required: true},
		}},
		{Name: "memory_recall", Description: "Retrieve information from persistent memory.", Parameters: []HubToolParam{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
		}},
		{Name: "memory_delete", Description: "Delete information from persistent memory.", Parameters: []HubToolParam{
			{Name: "key", Type: "string", Description: "Memory key", Required: true},
		}},

		// Cron (local)
		{Name: "cronjob_list", Description: "List all scheduled cron jobs.", Parameters: []HubToolParam{}},
		{Name: "cronjob_create", Description: "Create a new scheduled cron job.", Parameters: []HubToolParam{
			{Name: "id", Type: "string", Description: "Job ID", Required: true},
			{Name: "schedule", Type: "string", Description: "Cron expression", Required: true},
			{Name: "task", Type: "string", Description: "Task prompt", Required: true},
			{Name: "target", Type: "string", Description: "Push target", Required: false},
		}},
		{Name: "cronjob_delete", Description: "Delete a cron job by ID.", Parameters: []HubToolParam{
			{Name: "id", Type: "string", Description: "Job ID to delete", Required: true},
		}},

		// Skills (local)
		{Name: "skill_load", Description: "Load a skill plugin for this session.", Parameters: []HubToolParam{
			{Name: "name", Type: "string", Description: "Skill name", Required: true},
		}},

		// Utilities (local)
		{Name: "time", Description: "Get current date and time in UTC.", Parameters: []HubToolParam{}},
		{Name: "configure", Description: "Update a runtime agent setting.", Parameters: []HubToolParam{
			{Name: "field", Type: "string", Description: "Setting name", Required: true},
			{Name: "value", Type: "string", Description: "New value", Required: true},
		}},

		// Agent operations (require hub connection)
		{Name: "agent_delegate", Description: "Delegate a task to another agent in the hub.", Parameters: []HubToolParam{
			{Name: "id", Type: "string", Description: "Target agent ID", Required: true},
			{Name: "task", Type: "string", Description: "Task description", Required: true},
			{Name: "context", Type: "object", Description: "Optional context", Required: false},
		}},
		{Name: "agent_list", Description: "List all agents registered in the hub.", Parameters: []HubToolParam{}},
		{Name: "agent_get", Description: "Get details for a specific agent.", Parameters: []HubToolParam{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_status", Description: "Get status of a named agent.", Parameters: []HubToolParam{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_stop", Description: "Stop a running agent. Cannot stop self.", Parameters: []HubToolParam{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "agent_restart", Description: "Restart an agent. Cannot restart self.", Parameters: []HubToolParam{
			{Name: "name", Type: "string", Description: "Agent name", Required: true},
		}},
		{Name: "hub_status", Description: "Get hub information (name, JetStream status).", Parameters: []HubToolParam{}},
	}
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

// agentStatus returns the status of a named agent.
func (h *HubInbox) agentStatus(ctx context.Context, args map[string]any) ToolReply {
	name := stringArg(args, "name")
	if name == "" {
		return errorReply("agent_status", "param 'name' is required")
	}
	status, err := h.supervisor.Status(name)
	if err != nil {
		return errorReply("agent_status", err.Error())
	}
	return ToolReply{
		Tool:   "agent_status",
		OK:     true,
		Output: marshalJSON(status),
	}
}

// agentStop stops a named agent.
func (h *HubInbox) agentStop(ctx context.Context, args map[string]any) ToolReply {
	name := stringArg(args, "name")
	if name == "" {
		return errorReply("agent_stop", "param 'name' is required")
	}
	if err := h.supervisor.Stop(name); err != nil {
		return errorReply("agent_stop", err.Error())
	}
	return ToolReply{
		Tool:   "agent_stop",
		OK:     true,
		Output: name + " stopped",
	}
}

// agentRestart restarts a named agent.
func (h *HubInbox) agentRestart(ctx context.Context, args map[string]any) ToolReply {
	name := stringArg(args, "name")
	if name == "" {
		return errorReply("agent_restart", "param 'name' is required")
	}
	if err := h.supervisor.Restart(name); err != nil {
		return errorReply("agent_restart", err.Error())
	}
	return ToolReply{
		Tool:   "agent_restart",
		OK:     true,
		Output: name + " restarted",
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

// agentGet returns the registry entry for a named agent.
func (h *HubInbox) agentGet(ctx context.Context, args map[string]any) ToolReply {
	name := stringArg(args, "name")
	if name == "" {
		return errorReply("agent_get", "param 'name' is required")
	}
	entry, ok := h.registry.Get(name)
	if !ok {
		return errorReply("agent_get", "agent not found: "+name)
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
