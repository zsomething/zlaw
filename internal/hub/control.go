package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/config"
)

// ControlSupervisor is the subset of Supervisor needed by ControlSocket.
type ControlSupervisor interface {
	Statuses() []AgentStatus
	Status(name string) (AgentStatus, error)
	Stop(name string) error
	Restart(name string) error
	Spawn(ctx context.Context, entry config.AgentEntry) error
}

// ControlSocket provides a JSON-RPC-like interface over a Unix domain socket.
// It allows the hub CLI to query and control the running hub process.
//
// Request format (one JSON object per line):
//
//	{"method": "hub.status"}                    → HubStatusResponse
//	{"method": "agent.list"}                    → AgentListResponse
//	{"method": "agent.status", "params": {"name": "foo"}} → AgentStatusResponse
//	{"method": "agent.configure", "params": {"name": "foo", "key": "llm.model", "value": "claude-opus-4"}}
//	{"method": "agent.stop", "params": {"name": "foo"}}
//	{"method": "agent.restart", "params": {"name": "foo"}}
//	{"method": "agent.remove", "params": {"name": "foo"}}
//
// Response format:
//
//	{"ok": true, "result": <payload>}
//	{"ok": false, "error": "description"}
//
// All fields in the payload use json.RawMessage so the client can decode
// as needed without the server needing to know the full type set.
type ControlSocket struct {
	socketPath  string
	supervisor  ControlSupervisor
	registry    AgentRegistryReader
	mgmtHandler *ManagementHandler
	cfg         config.HubConfig
	logger      *slog.Logger

	mu       sync.Mutex
	ln       net.Listener
	done     chan struct{}
	sessions sync.Map // map[net.Conn]struct{} for graceful close tracking
}

const (
	controlSocketName = "control.sock"
	socketReadTimeout = 10 * time.Second
)

// NewControlSocket creates a ControlSocket that listens on socketPath.
// supervisor and registry are used to answer agent queries.
// mgmtHandler is used for agent lifecycle operations.
// cfg is the hub config used for static hub info (name, version, etc.).
func NewControlSocket(
	socketPath string,
	supervisor ControlSupervisor,
	registry AgentRegistryReader,
	mgmtHandler *ManagementHandler,
	cfg config.HubConfig,
	logger *slog.Logger,
) *ControlSocket {
	return &ControlSocket{
		socketPath:  socketPath,
		supervisor:  supervisor,
		registry:    registry,
		mgmtHandler: mgmtHandler,
		cfg:         cfg,
		logger:      logger,
		done:        make(chan struct{}),
	}
}

// Start listens on the socket and serves requests until ctx is cancelled.
// It is safe to call Start multiple times on the same socket (idempotent).
func (cs *ControlSocket) Start(ctx context.Context) error {
	cs.mu.Lock()
	ln, err := net.Listen("unix", cs.socketPath)
	if err != nil {
		cs.mu.Unlock()
		return fmt.Errorf("control socket listen %s: %w", cs.socketPath, err)
	}
	// Make the socket accessible to non-owner processes (e.g., zlaw CLI).
	if err := os.Chmod(cs.socketPath, 0o777); err != nil {
		ln.Close() //nolint:errcheck
		cs.mu.Unlock()
		return fmt.Errorf("chmod control socket: %w", err)
	}
	cs.ln = ln
	cs.mu.Unlock()

	cs.logger.Info("control socket listening", "path", cs.socketPath)

	go cs.serve(ctx)
	return nil
}

// serve accepts connections and handles each in a dedicated goroutine.
func (cs *ControlSocket) serve(ctx context.Context) {
	for {
		conn, err := cs.ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-cs.done:
				return
			default:
				// EINVAL after Close, just exit
				return
			}
		}

		session := conn
		cs.sessions.Store(session, struct{}{})

		go func() {
			defer func() {
				cs.sessions.Delete(session)
				session.Close() //nolint:errcheck
			}()
			cs.handleConn(ctx, session)
		}()
	}
}

// handleConn reads newline-delimited JSON requests from conn and writes responses.
func (cs *ControlSocket) handleConn(ctx context.Context, conn net.Conn) {
	dec := json.NewDecoder(conn)
	for {
		setDeadline(conn, socketReadTimeout)

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return // EOF or parse error
		}

		var req ControlRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			writeError(conn, "invalid request format")
			continue
		}

		result, err := cs.dispatch(ctx, req)
		if err != nil {
			writeError(conn, err.Error())
			continue
		}

		writeOK(conn, result)
	}
}

// dispatch routes a single request to the appropriate handler.
func (cs *ControlSocket) dispatch(ctx context.Context, req ControlRequest) (json.RawMessage, error) {
	switch req.Method {
	case "hub.status":
		return cs.hubStatus()
	case "agent.list":
		return cs.agentList()
	case "agent.status":
		return cs.agentStatus(req.Params)
	case "agent.configure":
		return nil, cs.agentConfigure(ctx, req.Params)
	case "agent.stop":
		return nil, cs.agentStop(req.Params)
	case "agent.restart":
		return nil, cs.agentRestart(req.Params)
	case "agent.remove":
		return nil, cs.agentRemove(req.Params)
	default:
		return nil, fmt.Errorf("unknown method %q", req.Method)
	}
}

// ── Request / Response types ──────────────────────────────────────────────────

// ControlRequest is the parsed incoming request from a JSON object.
type ControlRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     json.RawMessage `json:"id,omitempty"` // optional; echoed in response
}

// ControlResponseOK is the success response envelope.
type ControlResponseOK struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	ID     json.RawMessage `json:"id,omitempty"`
}

// ControlResponseError is the error response envelope.
type ControlResponseError struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error"`
	ID    json.RawMessage `json:"id,omitempty"`
}

// hubStatusResult is the payload for hub.status.
type hubStatusResult struct {
	Name       string        `json:"name"`
	Version    string        `json:"version"`
	AgentCount int           `json:"agent_count"`
	Agents     []AgentStatus `json:"agents"`
	ConnStatus string        `json:"connected_status"`
	NATS       *NATSStatus   `json:"nats,omitempty"`
}

// NATSStatus holds NATS connection info for hub.status.
type NATSStatus struct {
	Listen    string `json:"listen"`
	JetStream bool   `json:"jetstream"`
}

// hubStatus returns a snapshot of hub state.
func (cs *ControlSocket) hubStatus() (json.RawMessage, error) {
	statuses := cs.supervisor.Statuses()
	agents := append([]AgentStatus(nil), statuses...)

	connStatus := "standalone"
	var natsStatus *NATSStatus
	if cs.cfg.NATS.Listen != "" {
		connStatus = "nats"
		natsStatus = &NATSStatus{
			Listen:    cs.cfg.NATS.Listen,
			JetStream: cs.cfg.NATS.JetStream,
		}
	}

	res := hubStatusResult{
		Name:       cs.cfg.Hub.Name,
		AgentCount: len(agents),
		Agents:     agents,
		ConnStatus: connStatus,
		NATS:       natsStatus,
	}
	return json.Marshal(res)
}

// agentListResult is the payload for agent.list.
type agentListResult struct {
	Agents []RegistryEntry `json:"agents"`
}

// agentList returns all registered agents.
func (cs *ControlSocket) agentList() (json.RawMessage, error) {
	entries := cs.registry.List()
	res := agentListResult{Agents: entries}
	return json.Marshal(res)
}

// agentStatus returns the status of a named agent.
func (cs *ControlSocket) agentStatus(params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("param 'name' is required")
	}

	status, err := cs.supervisor.Status(p.Name)
	if err != nil {
		return nil, err
	}
	return json.Marshal(status)
}

// agentConfigure updates a runtime field for a named agent.
func (cs *ControlSocket) agentConfigure(ctx context.Context, params json.RawMessage) error {
	var p struct {
		Name  string `json:"name"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return fmt.Errorf("parse params: %w", err)
	}
	if p.Name == "" || p.Key == "" || p.Value == "" {
		return fmt.Errorf("params 'name', 'key', 'value' are required")
	}

	agentDir := filepath.Join(config.ZlawHome(), "agents", p.Name)
	if err := config.WriteRuntimeFieldToDir(agentDir, p.Key, p.Value); err != nil {
		return fmt.Errorf("agent.configure: %w", err)
	}
	return nil
}

// agentStop stops a named agent.
func (cs *ControlSocket) agentStop(params json.RawMessage) error {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return fmt.Errorf("parse params: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("param 'name' is required")
	}
	return cs.supervisor.Stop(p.Name)
}

// agentRestart restarts a named agent.
func (cs *ControlSocket) agentRestart(params json.RawMessage) error {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return fmt.Errorf("parse params: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("param 'name' is required")
	}
	return cs.supervisor.Restart(p.Name)
}

// agentRemove stops and deregisters a named agent.
func (cs *ControlSocket) agentRemove(params json.RawMessage) error {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return fmt.Errorf("parse params: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("param 'name' is required")
	}

	if err := cs.supervisor.Stop(p.Name); err != nil {
		cs.logger.Warn("control: stop before remove failed", "name", p.Name, "err", err)
	}
	cs.registry.Deregister(p.Name)
	return nil
}

// writeOK marshals and sends a success response.
func writeOK(conn net.Conn, result json.RawMessage) {
	resp := ControlResponseOK{OK: true, Result: result}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(socketWriteDeadline)) //nolint:errcheck
	conn.Write(data)                                           //nolint:errcheck
	conn.Write([]byte("\n"))                                   //nolint:errcheck
}

// writeError marshals and sends an error response.
func writeError(conn net.Conn, errMsg string) {
	resp := ControlResponseError{OK: false, Error: errMsg}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	conn.SetWriteDeadline(time.Now().Add(socketWriteDeadline)) //nolint:errcheck
	conn.Write(data)                                           //nolint:errcheck
	conn.Write([]byte("\n"))                                   //nolint:errcheck
}

const socketWriteDeadline = 5 * time.Second

// setDeadline sets a read deadline on conn, ignoring errors.
func setDeadline(conn net.Conn, d time.Duration) {
	conn.SetReadDeadline(time.Now().Add(d)) //nolint:errcheck
}

// Stop closes the control socket listener and all active sessions.
func (cs *ControlSocket) Stop() error {
	close(cs.done)

	cs.mu.Lock()
	defer cs.mu.Unlock()

	var errs []error
	if cs.ln != nil {
		if err := cs.ln.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	cs.sessions.Range(func(k, _ any) bool {
		if c, ok := k.(net.Conn); ok {
			c.Close() //nolint:errcheck
		}
		return true
	})

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
