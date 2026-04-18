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

// ToolInfo exposes hub-level tool metadata for the web UI.
type ToolInfo struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  []ParamInfo `json:"parameters"`
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

// handleTools returns hub-level built-in tools as JSON.
func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	tools := []ToolInfo{
		{Name: "hub_status", Description: "Returns static hub information including name, JetStream status, and routing configuration.", Parameters: []ParamInfo{}},
		{Name: "agent_list", Description: "Lists all registered agents in the hub with their registry entries.", Parameters: []ParamInfo{}},
		{Name: "agent_status", Description: "Returns the current status of a named agent (running state, PID, last heartbeat).", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to check", Required: true},
		}},
		{Name: "get_agent", Description: "Returns the full registry entry for a named agent (capabilities, version, config path).", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to retrieve", Required: true},
		}},
		{Name: "agent_stop", Description: "Stops a running agent by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to stop", Required: true},
		}},
		{Name: "agent_restart", Description: "Restarts a stopped or running agent by name.", Parameters: []ParamInfo{
			{Name: "name", Type: "string", Description: "Name of the agent to restart", Required: true},
		}},
	}
	s.writeJSON(w, tools)
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
