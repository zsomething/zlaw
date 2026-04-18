package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

// Server is an HTTP server that serves the read-only hub web UI.
type Server struct {
	log   *slog.Logger
	state State
	mux   *http.ServeMux
	addr  string
}

// ToolInfo exposes tool metadata for the web UI.
type ToolInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  []ParamInfo `json:"parameters"`
	EnabledFor  []string    `json:"enabled_for"` // agent names that have this tool enabled
}

// ParamInfo describes a tool parameter.
type ParamInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// State exposes read-only hub state for the web UI.
type State interface {
	HubConfig() config.HubConfig
	NATSAddr() string
	Agents() []AgentInfo
	AuditEntries(limit int, eventType string) ([]hub.AuditEntry, error)
}

// AgentInfo merges registry and process state for display.
type AgentInfo struct {
	hub.RegistryEntry
	PID     int    `json:"pid"`
	Running bool   `json:"running"`
	LastErr string `json:"last_err,omitempty"`
}

// NewServer creates an HTTP server bound to addr with the given state.
func NewServer(addr string, state State, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	mux := http.NewServeMux()
	srv := &Server{
		log:   log,
		state: state,
		mux:   mux,
		addr:  addr,
	}

	mux.Handle("GET /", http.HandlerFunc(srv.handleIndex))
	mux.Handle("GET /tools", http.HandlerFunc(srv.handleToolsPage))
	mux.Handle("GET /audit", http.HandlerFunc(srv.handleAuditPage))
	mux.Handle("GET /api/hub", http.HandlerFunc(srv.handleHub))
	mux.Handle("GET /api/agents", http.HandlerFunc(srv.handleAgents))
	mux.Handle("GET /api/tools", http.HandlerFunc(srv.handleTools))
	mux.Handle("GET /api/audit", http.HandlerFunc(srv.handleAudit))

	return srv
}

// Addr returns the server's listen address.
func (s *Server) Addr() string { return s.addr }

// Start runs the server in a new goroutine and returns immediately.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		s.log.Info("web: listening", "addr", s.addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Warn("web: server error", "err", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	srv := &http.Server{Addr: s.addr, Handler: s.mux}
	return srv.Shutdown(ctx)
}

// handleIndex serves the main HTML page.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	cfg := s.state.HubConfig()
	data := pongo2.Context{
		"HubName":        cfg.Hub.Name,
		"HubDescription": cfg.Hub.Description,
		"NATSAddr":       s.state.NATSAddr(),
		"Agents":         s.state.Agents(),
	}
	s.serveTemplate(w, "templates/pages/index.html", data)
}

// handleAuditPage serves the audit log HTML page.
func (s *Server) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "templates/pages/audit.html", pongo2.Context{})
}

// handleToolsPage serves the hub tools HTML page.
func (s *Server) handleToolsPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "templates/pages/tools.html", pongo2.Context{})
}

// handleHub returns hub identity and NATS status as JSON.
func (s *Server) handleHub(w http.ResponseWriter, r *http.Request) {
	cfg := s.state.HubConfig()
	data := hubStatus{
		Name:         cfg.Hub.Name,
		Description:  cfg.Hub.Description,
		NATSAddr:     s.state.NATSAddr(),
		AuditLogPath: cfg.Hub.AuditLogPath,
	}
	s.writeJSON(w, data)
}

// handleAgents returns agent list as JSON. Query param ?name= restricts to single agent.
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name != "" {
		for _, a := range s.state.Agents() {
			if a.Name == name {
				s.writeJSON(w, a)
				return
			}
		}
		http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
		return
	}
	s.writeJSON(w, s.state.Agents())
}

// handleTools returns all built-in tools as JSON.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	agents := s.state.Agents()
	agentNames := make([]string, len(agents))
	for i, a := range agents {
		agentNames[i] = a.Name
	}

	// Agent built-in tools (local execution by each agent)
	agentTools := []ToolInfo{
		{Name: "bash", Description: "Execute shell commands in the agent's workspace. Use for file operations, git, npm, or any system task.", Parameters: []ParamInfo{
			{Name: "command", Type: "string", Description: "Shell command to execute", Required: true},
			{Name: "cwd", Type: "string", Description: "Working directory (defaults to agent workspace)", Required: false},
			{Name: "timeout", Type: "number", Description: "Timeout in seconds (default: 60)", Required: false},
		}},
		{Name: "read_file", Description: "Read the contents of a file from the agent's workspace.", Parameters: []ParamInfo{
			{Name: "path", Type: "string", Description: "Relative path to file in workspace", Required: true},
		}},
		{Name: "write_file", Description: "Create or overwrite a file in the agent's workspace.", Parameters: []ParamInfo{
			{Name: "path", Type: "string", Description: "Relative path for the new file", Required: true},
			{Name: "content", Type: "string", Description: "File content to write", Required: true},
		}},
		{Name: "edit_file", Description: "Make targeted edits to an existing file using line-based replacements.", Parameters: []ParamInfo{
			{Name: "path", Type: "string", Description: "Path to file to edit", Required: true},
			{Name: "old_text", Type: "string", Description: "Exact text to replace", Required: true},
			{Name: "new_text", Type: "string", Description: "Replacement text", Required: true},
		}},
		{Name: "glob", Description: "Find files matching a glob pattern in the agent's workspace.", Parameters: []ParamInfo{
			{Name: "pattern", Type: "string", Description: "Glob pattern (e.g., **/*.go)", Required: true},
		}},
		{Name: "grep", Description: "Search for text within files using regex.", Parameters: []ParamInfo{
			{Name: "pattern", Type: "string", Description: "Regex pattern to search", Required: true},
			{Name: "path", Type: "string", Description: "Directory or file path to search", Required: false},
			{Name: "file_pattern", Type: "string", Description: "File glob pattern (e.g., *.go)", Required: false},
		}},
		{Name: "web_fetch", Description: "Fetch content from a URL.", Parameters: []ParamInfo{
			{Name: "url", Type: "string", Description: "URL to fetch", Required: true},
			{Name: "prompt", Type: "string", Description: "Extract specific information from the page", Required: false},
		}},
		{Name: "web_search", Description: "Search the web for information.", Parameters: []ParamInfo{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
			{Name: "top_n", Type: "number", Description: "Number of results (default: 5)", Required: false},
		}},
		{Name: "http_request", Description: "Make HTTP requests with full control over method, headers, and body.", Parameters: []ParamInfo{
			{Name: "method", Type: "string", Description: "HTTP method (GET, POST, PUT, DELETE, etc.)", Required: true},
			{Name: "url", Type: "string", Description: "Request URL", Required: true},
			{Name: "headers", Type: "object", Description: "Request headers", Required: false},
			{Name: "body", Type: "string", Description: "Request body", Required: false},
		}},
		{Name: "memory_save", Description: "Store information in the agent's persistent memory.", Parameters: []ParamInfo{
			{Name: "key", Type: "string", Description: "Memory key (use snake_case)", Required: true},
			{Name: "value", Type: "string", Description: "Value to store", Required: true},
		}},
		{Name: "memory_recall", Description: "Retrieve information from the agent's persistent memory.", Parameters: []ParamInfo{
			{Name: "key", Type: "string", Description: "Memory key to retrieve", Required: true},
		}},
		{Name: "memory_delete", Description: "Delete information from the agent's persistent memory.", Parameters: []ParamInfo{
			{Name: "key", Type: "string", Description: "Memory key to delete", Required: true},
		}},
		{Name: "delegate", Description: "Delegate a task to another registered agent in the hub.", Parameters: []ParamInfo{
			{Name: "agent", Type: "string", Description: "Target agent name", Required: true},
			{Name: "task", Type: "string", Description: "Task description for the agent", Required: true},
		}},
		{Name: "list_cronjobs", Description: "List all scheduled cron jobs for this agent.", Parameters: []ParamInfo{}},
		{Name: "create_cronjob", Description: "Create a new scheduled cron job.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Job name", Required: true},
			{Name: "schedule", Type: "string", Description: "Cron expression (e.g., 0 * * * *)", Required: true},
			{Name: "task", Type: "string", Description: "Task description", Required: true},
		}},
		{Name: "delete_cronjob", Description: "Delete a scheduled cron job by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Job name to delete", Required: true},
		}},
		{Name: "skill_load", Description: "Load and activate a skill plugin for this session.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Skill name to load", Required: true},
		}},
		{Name: "current_time", Description: "Get the current date and time.", Parameters: []ParamInfo{}},
	}

	// Hub tools (routed via hub NATS inbox)
	hubTools := []ToolInfo{
		{Name: "hub_status", Description: "Returns static hub information including name, JetStream status, and routing configuration.", Parameters: []ParamInfo{}},
		{Name: "agent_list", Description: "Lists all registered agents in the hub with their registry entries.", Parameters: []ParamInfo{}},
		{Name: "agent_get", Description: "Get details for a specific agent by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to retrieve", Required: true},
		}},
		{Name: "agent_status", Description: "Returns the current status of a named agent (running state, PID, last heartbeat).", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to check", Required: true},
		}},
		{Name: "agent_stop", Description: "Stops a running agent by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to stop", Required: true},
		}},
		{Name: "agent_restart", Description: "Restarts a stopped or running agent by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to restart", Required: true},
		}},
	}

	// Set EnabledFor for agent tools
	for i := range agentTools {
		agentTools[i].EnabledFor = agentNames
	}

	// Set EnabledFor for hub tools
	for i := range hubTools {
		hubTools[i].EnabledFor = agentNames
	}

	s.writeJSON(w, map[string]any{
		"agent_tools": agentTools,
		"hub_tools":   hubTools,
	})
}

// handleAudit returns recent audit entries as JSON.
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	eventType := r.URL.Query().Get("type")

	entries, err := s.state.AuditEntries(limit, eventType)
	if err != nil {
		http.Error(w, `{"error":"failed to read audit log"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, entries)
}

// hubStatus is the JSON response for /api/hub.
type hubStatus struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	NATSAddr     string `json:"nats_addr"`
	AuditLogPath string `json:"audit_log_path"`
}

func (s *Server) serveTemplate(w http.ResponseWriter, t string, data pongo2.Context) {
	if err := executeTemplate(w, t, data); err != nil {
		s.log.Warn("web: template error", "template", t, "err", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
