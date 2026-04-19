package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/tools"
)

// Server is an HTTP server that serves the read-only hub web UI.
type Server struct {
	log   *slog.Logger
	state State
	r     *chi.Mux
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
	// Sessions returns a list of sessions for a given agent ID.
	Sessions(agentID string) ([]SessionInfo, error)
	// SessionMessages returns all messages for a given agent and session.
	SessionMessages(agentID, sessionID string) ([]MessageInfo, error)
	// CompiledContext returns the assembled context for a given agent and session.
	CompiledContext(agentID, sessionID string) (ContextInfo, error)
	// WorkspaceFiles returns the workspace files for a given agent.
	WorkspaceFiles(agentID string) ([]FileInfo, error)
	// AgentSkills returns skills for a given agent.
	AgentSkills(agentID string) ([]SkillInfo, error)
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
	AgentID      string    `json:"agent_id"`
	Channel      string    `json:"channel"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	Title        string    `json:"title"`
	Active       bool      `json:"active"`
}

// MessageInfo represents a single message in a session.
type MessageInfo struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
	TokensIn  int    `json:"tokens_in,omitempty"`
	TokensOut int    `json:"tokens_out,omitempty"`
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
	srv := &Server{
		log:   log,
		state: state,
		addr:  addr,
	}
	srv.r = srv.routes()
	return srv
}

func (s *Server) routes() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// HTML pages
	r.Get("/", s.handleIndex)
	r.Get("/tools", s.handleToolsPage)
	r.Get("/agents", s.handleAgentsPage)
	r.Get("/agents/{agentID}", s.handleAgentDetailPage)
	r.Get("/audit", s.handleAuditPage)

	// API
	r.Route("/api", func(r chi.Router) {
		r.Get("/hub", s.handleHub)
		r.Get("/agents", s.handleAgents)
		r.Route("/agents/{agentID}", func(r chi.Router) {
			r.Get("/", s.handleAgentGet)
			r.Get("/tools", s.handleAgentTools)
			r.Get("/sessions", s.handleAgentSessions)
			r.Get("/sessions/{sessionID}", s.handleSessionDetail)
			r.Get("/files", s.handleAgentFiles)
			r.Get("/skills", s.handleAgentSkills)
		})
		r.Get("/tools", s.handleTools)
		r.Get("/audit", s.handleAudit)
	})

	return r
}

// Addr returns the server's listen address.
func (s *Server) Addr() string { return s.addr }

// Start runs the server in a new goroutine and returns immediately.
func (s *Server) Start(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.addr,
		Handler:      s.r,
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
	srv := &http.Server{Addr: s.addr, Handler: s.r}
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
	s.serveTemplate(w, "index.html", data)
}

// handleAuditPage serves the audit log HTML page.
func (s *Server) handleAuditPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "audit.html", pongo2.Context{})
}

// handleToolsPage serves the hub tools HTML page.
func (s *Server) handleToolsPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "tools.html", pongo2.Context{})
}

// handleAgentsPage serves the agents overview HTML page.
func (s *Server) handleAgentsPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "agents.html", pongo2.Context{})
}

// handleAgentDetailPage serves the agent detail HTML page.
func (s *Server) handleAgentDetailPage(w http.ResponseWriter, r *http.Request) {
	s.serveTemplate(w, "agent_detail.html", pongo2.Context{})
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

// handleAgentGet returns a single agent by ID.
func (s *Server) handleAgentGet(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	for _, a := range s.state.Agents() {
		if a.Name == agentID {
			s.writeJSON(w, a)
			return
		}
	}
	http.Error(w, `{"error":"agent not found"}`, http.StatusNotFound)
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

// handleAgentTools returns tools for a specific agent as JSON.
func (s *Server) handleAgentTools(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	var caps []string
	for _, a := range s.state.Agents() {
		if a.Name == agentID {
			caps = a.Capabilities
			break
		}
	}
	allTools := s.state.Tools()
	var filtered []tools.Definition
	for _, t := range allTools {
		if len(caps) == 0 {
			filtered = append(filtered, t)
		} else {
			for _, c := range caps {
				if c == t.Name {
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
	agentID := chi.URLParam(r, "agentID")
	sessions, err := s.state.Sessions(agentID)
	if err != nil {
		http.Error(w, `{"error":"failed to load sessions"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, sessions)
}

// handleSessionDetail returns messages and context for a session.
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	sessionID := chi.URLParam(r, "sessionID")

	view := r.URL.Query().Get("view")
	if view == "context" {
		ctx, err := s.state.CompiledContext(agentID, sessionID)
		if err != nil {
			http.Error(w, `{"error":"failed to load context"}`, http.StatusInternalServerError)
			return
		}
		s.writeJSON(w, ctx)
		return
	}

	messages, err := s.state.SessionMessages(agentID, sessionID)
	if err != nil {
		http.Error(w, `{"error":"failed to load messages"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{"messages": messages})
}

// handleAgentFiles returns workspace files for a specific agent as JSON.
func (s *Server) handleAgentFiles(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	files, err := s.state.WorkspaceFiles(agentID)
	if err != nil {
		http.Error(w, `{"error":"failed to load files"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, files)
}

// handleAgentSkills returns skills for a specific agent as JSON.
func (s *Server) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentID")
	skills, err := s.state.AgentSkills(agentID)
	if err != nil {
		http.Error(w, `{"error":"failed to load skills"}`, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, skills)
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
