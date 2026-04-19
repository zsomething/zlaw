package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/tools"
)

// Server is an HTTP server that serves the read-only hub web UI.
type Server struct {
	log   *slog.Logger
	state State
	mux   *http.ServeMux
	addr  string
}

// State exposes read-only hub state for the web UI.
type State interface {
	HubConfig() config.HubConfig
	NATSAddr() string
	Agents() []AgentInfo
	AuditEntries(limit int, eventType string) ([]hub.AuditEntry, error)
	// Tools returns all built-in tool definitions.
	Tools() []tools.Definition
	// Sessions returns a list of sessions for a given agent name.
	Sessions(agentName string) ([]SessionInfo, error)
	// SessionMessages returns all messages for a given agent and session.
	SessionMessages(agentName, sessionID string) ([]MessageInfo, error)
	// CompiledContext returns the assembled context for a given agent and session.
	CompiledContext(agentName, sessionID string) (ContextInfo, error)
	// WorkspaceFiles returns the workspace files for a given agent.
	WorkspaceFiles(agentName string) ([]FileInfo, error)
	// AgentSkills returns skills for a given agent.
	AgentSkills(agentName string) ([]SkillInfo, error)
}

// AgentInfo merges registry and process state for display.
type AgentInfo struct {
	hub.RegistryEntry
	PID     int    `json:"pid"`
	Running bool   `json:"running"`
	LastErr string `json:"last_err,omitempty"`
}

// SessionInfo holds lightweight session metadata for display.
type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	AgentName    string    `json:"agent_name"`
	Channel      string    `json:"channel"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	Title        string    `json:"title"`
	Active       bool      `json:"active"`
}

// MessageInfo represents a single message in a session.
type MessageInfo struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	TokensIn  int       `json:"tokens_in"`
	TokensOut int       `json:"tokens_out"`
}

// ContextInfo holds the compiled context for an agent session.
type ContextInfo struct {
	SystemPrompt string   `json:"system_prompt"`
	ToolDefs     []string `json:"tool_definitions"`
	Memories     []string `json:"memories"`
	RecentMsgs   int      `json:"recent_messages"`
	SessionVars  []string `json:"session_variables"`
}

// FileInfo represents a workspace file.
type FileInfo struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	IsDir    bool      `json:"is_dir"`
	Masked   bool      `json:"masked,omitempty"`
}

// SkillInfo represents a skill for display.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
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
	mux.Handle("GET /agents", http.HandlerFunc(srv.handleAgentsPage))
	mux.Handle("GET /agents/tools", http.HandlerFunc(srv.handleAgentToolsPage))
	mux.Handle("GET /agents/sessions", http.HandlerFunc(srv.handleAgentSessionsPage))
	mux.Handle("GET /agents/files", http.HandlerFunc(srv.handleAgentFilesPage))
	mux.Handle("GET /agents/skills", http.HandlerFunc(srv.handleAgentSkillsPage))
	mux.Handle("GET /audit", http.HandlerFunc(srv.handleAuditPage))
	mux.Handle("GET /api/hub", http.HandlerFunc(srv.handleHub))
	mux.Handle("GET /api/agents", http.HandlerFunc(srv.handleAgents))
	mux.Handle("GET /api/agents/tools", http.HandlerFunc(srv.handleAgentTools))
	mux.Handle("GET /api/agents/sessions", http.HandlerFunc(srv.handleAgentSessions))
	mux.Handle("GET /api/agents/sessions/", http.HandlerFunc(srv.handleSessionDetail))
	mux.Handle("GET /api/agents/files", http.HandlerFunc(srv.handleAgentFiles))
	mux.Handle("GET /api/agents/skills", http.HandlerFunc(srv.handleAgentSkills))
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
	tools := s.state.Tools()
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	s.writeJSON(w, map[string]any{"tools": tools})
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

// handleAgentsPage serves the agents overview HTML page.
func (s *Server) handleAgentsPage(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	data := pongo2.Context{
		"SelectedAgent": agent,
	}
	s.serveTemplate(w, "templates/pages/agents.html", data)
}

// handleAgentToolsPage serves the per-agent tools HTML page.
func (s *Server) handleAgentToolsPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "templates/pages/agent_tools.html", pongo2.Context{})
}

// handleAgentSessionsPage serves the agent sessions HTML page.
func (s *Server) handleAgentSessionsPage(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	data := pongo2.Context{
		"SelectedAgent": agent,
	}
	s.serveTemplate(w, "templates/pages/agent_sessions.html", data)
}

// handleAgentFilesPage serves the agent workspace files HTML page.
func (s *Server) handleAgentFilesPage(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	data := pongo2.Context{
		"SelectedAgent": agent,
	}
	s.serveTemplate(w, "templates/pages/agent_files.html", data)
}

// handleAgentSkillsPage serves the agent skills HTML page.
func (s *Server) handleAgentSkillsPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "templates/pages/agent_skills.html", pongo2.Context{})
}

// handleAgentTools returns tools for a specific agent as JSON.
func (s *Server) handleAgentTools(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		http.Error(w, `{"error":"agent parameter required"}`, http.StatusBadRequest)
		return
	}
	// Get agent capabilities from registry
	var caps []string
	for _, a := range s.state.Agents() {
		if a.Name == agent {
			caps = a.Capabilities
			break
		}
	}
	// Filter global tools by agent capabilities
	allTools := s.state.Tools()
	var filtered []tools.Definition
	for _, t := range allTools {
		if len(caps) == 0 {
			filtered = append(filtered, t) // no caps = all tools
		} else {
			// Check if tool name is in capabilities
			for _, c := range caps {
				if c == t.Name || strings.HasPrefix(c, "tool:") {
					filtered = append(filtered, t)
					break
				}
			}
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	s.writeJSON(w, map[string]any{"tools": filtered})
}

// handleAgentSessions returns sessions for a specific agent as JSON.
func (s *Server) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		http.Error(w, `{"error":"agent parameter required"}`, http.StatusBadRequest)
		return
	}
	sessions, err := s.state.Sessions(agent)
	if err != nil {
		http.Error(w, `{"error":"failed to load sessions"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, sessions)
}

// handleSessionDetail returns messages and context for a specific session.
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	// Extract agent and session from path: /api/agents/sessions/<sessionID>?agent=<agent>
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/sessions/")
	parts := strings.SplitN(path, "/", 2)
	sessionID := parts[0]
	agent := r.URL.Query().Get("agent")

	if agent == "" || sessionID == "" {
		http.Error(w, `{"error":"agent and session parameters required"}`, http.StatusBadRequest)
		return
	}

	view := r.URL.Query().Get("view")
	if view == "context" {
		ctx, err := s.state.CompiledContext(agent, sessionID)
		if err != nil {
			http.Error(w, `{"error":"failed to load context"}`, http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, ctx)
		return
	}

	messages, err := s.state.SessionMessages(agent, sessionID)
	if err != nil {
		http.Error(w, `{"error":"failed to load messages"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{"messages": messages})
}

// handleAgentFiles returns workspace files for a specific agent as JSON.
func (s *Server) handleAgentFiles(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		http.Error(w, `{"error":"agent parameter required"}`, http.StatusBadRequest)
		return
	}
	files, err := s.state.WorkspaceFiles(agent)
	if err != nil {
		http.Error(w, `{"error":"failed to load files"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, files)
}

// handleAgentSkills returns skills for a specific agent as JSON.
func (s *Server) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	agent := r.URL.Query().Get("agent")
	if agent == "" {
		http.Error(w, `{"error":"agent parameter required"}`, http.StatusBadRequest)
		return
	}
	skills, err := s.state.AgentSkills(agent)
	if err != nil {
		http.Error(w, `{"error":"failed to load skills"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, skills)
}
