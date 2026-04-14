package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nats-io/nats.go"
	"github.com/zsomething/zlaw/internal/config"
)

const (
	// managementSubject is the NATS subject the hub listens on for management requests.
	managementSubject = "zlaw.hub.inbox"

	// scaffoldAgentTOML is the minimal agent.toml written when creating a new agent.
	scaffoldAgentTOML = `[agent]
id = %q
description = ""

[llm]
backend = ""
model = ""
auth_profile = ""
max_tokens = 4096
timeout_sec = 60

[tools]
allowed = []

[adapter]
type = "cli"
`
)

// ManagementRequest is the envelope for hub management requests.
type ManagementRequest struct {
	Op      string         `json:"op"`
	Params  map[string]any `json:"params"`
	ReplyTo string         `json:"reply_to"`
}

// ManagementReply is published back to ManagementRequest.ReplyTo.
type ManagementReply struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// AgentSpawner is the Supervisor subset used by ManagementHandler.
type AgentSpawner interface {
	Spawn(ctx context.Context, entry config.AgentEntry) error
	Stop(name string) error
	Restart(name string) error
}

// AgentRegistryReader is the Registry subset used by ManagementHandler.
type AgentRegistryReader interface {
	List() []RegistryEntry
	Deregister(name string)
}

// ManagementHandler subscribes to zlaw.hub.inbox and dispatches management
// operations (agent.list, agent.create, agent.configure, agent.stop,
// agent.restart, agent.remove) to the Supervisor and Registry.
type ManagementHandler struct {
	conn        *nats.Conn
	supervisor  AgentSpawner
	registry    AgentRegistryReader
	managerName string // name of the manager agent (self-protection)
	zlawHome    string
	logger      *slog.Logger
}

// NewManagementHandler creates a ManagementHandler. managerName is the name of
// the manager agent; agent.remove will reject requests targeting it.
func NewManagementHandler(
	conn *nats.Conn,
	supervisor AgentSpawner,
	registry AgentRegistryReader,
	managerName string,
	zlawHome string,
	logger *slog.Logger,
) *ManagementHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ManagementHandler{
		conn:        conn,
		supervisor:  supervisor,
		registry:    registry,
		managerName: managerName,
		zlawHome:    zlawHome,
		logger:      logger,
	}
}

// Start subscribes to zlaw.hub.inbox and processes incoming requests until ctx
// is cancelled.
func (h *ManagementHandler) Start(ctx context.Context) error {
	sub, err := h.conn.Subscribe(managementSubject, func(msg *nats.Msg) {
		h.handle(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("hub inbox: subscribe to %s: %w", managementSubject, err)
	}

	h.logger.Info("hub management inbox active", "subject", managementSubject)

	<-ctx.Done()
	sub.Unsubscribe() //nolint:errcheck
	return nil
}

// handle parses and dispatches a single management request.
// replyTo is resolved from the JSON body first, then from the NATS msg.Reply
// (set automatically by nc.Request on the sender side).
func (h *ManagementHandler) handle(ctx context.Context, msg *nats.Msg) {
	var req ManagementRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		h.logger.Warn("hub inbox: malformed request", "err", err)
		return
	}
	if req.Op == "" {
		h.logger.Warn("hub inbox: request missing op")
		return
	}

	// Prefer explicit reply_to in the envelope; fall back to NATS reply subject.
	replyTo := req.ReplyTo
	if replyTo == "" {
		replyTo = msg.Reply
	}
	if replyTo == "" {
		h.logger.Warn("hub inbox: no reply_to and no NATS reply subject", "op", req.Op)
		return
	}

	h.logger.Info("hub inbox: handling op", "op", req.Op)

	result, err := h.dispatch(ctx, req.Op, req.Params)

	reply := ManagementReply{OK: err == nil, Result: result}
	if err != nil {
		reply.Error = err.Error()
		h.logger.Warn("hub inbox: op failed", "op", req.Op, "err", err)
	} else {
		h.logger.Info("hub inbox: op succeeded", "op", req.Op)
	}

	replyData, marshalErr := json.Marshal(reply)
	if marshalErr != nil {
		h.logger.Error("hub inbox: marshal reply failed", "err", marshalErr)
		return
	}
	if pubErr := h.conn.Publish(replyTo, replyData); pubErr != nil {
		h.logger.Error("hub inbox: publish reply failed", "reply_to", replyTo, "err", pubErr)
	}
}

// dispatch routes req.Op to the appropriate handler.
func (h *ManagementHandler) dispatch(ctx context.Context, op string, params map[string]any) (any, error) {
	switch op {
	case "agent.list":
		return h.opAgentList()
	case "agent.create":
		return nil, h.opAgentCreate(ctx, params)
	case "agent.configure":
		return nil, h.opAgentConfigure(params)
	case "agent.stop":
		return nil, h.opAgentStop(params)
	case "agent.restart":
		return nil, h.opAgentRestart(params)
	case "agent.remove":
		return nil, h.opAgentRemove(params)
	default:
		return nil, fmt.Errorf("unknown op %q", op)
	}
}

// opAgentList returns all entries from the registry.
func (h *ManagementHandler) opAgentList() (any, error) {
	return h.registry.List(), nil
}

// opAgentCreate scaffolds a new agent directory and spawns the agent.
func (h *ManagementHandler) opAgentCreate(ctx context.Context, params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.create: param 'name' is required")
	}

	agentDir := filepath.Join(h.zlawHome, "agents", name)
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return fmt.Errorf("agent.create: create agent dir %s: %w", agentDir, err)
	}

	tomlPath := filepath.Join(agentDir, "agent.toml")
	if _, err := os.Stat(tomlPath); os.IsNotExist(err) {
		content := fmt.Sprintf(scaffoldAgentTOML, name)
		if err := os.WriteFile(tomlPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("agent.create: write agent.toml: %w", err)
		}
	}

	entry := config.AgentEntry{
		Name:          name,
		Dir:           agentDir,
		RestartPolicy: config.RestartOnFailure,
	}
	if bin, ok := stringParam(params, "binary"); ok {
		entry.Binary = bin
	}
	if dir, ok := stringParam(params, "dir"); ok && dir != "" {
		entry.Dir = dir
	}
	if mgr, ok := params["manager"].(bool); ok {
		entry.Manager = mgr
	}

	if err := h.supervisor.Spawn(ctx, entry); err != nil {
		return fmt.Errorf("agent.create: spawn: %w", err)
	}

	h.logger.Info("hub inbox: agent created", "name", name, "dir", entry.Dir)
	return nil
}

// opAgentConfigure writes a key/value pair to the agent's runtime.toml.
func (h *ManagementHandler) opAgentConfigure(params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.configure: param 'name' is required")
	}
	key, ok := stringParam(params, "key")
	if !ok || key == "" {
		return fmt.Errorf("agent.configure: param 'key' is required")
	}
	value, ok := stringParam(params, "value")
	if !ok {
		return fmt.Errorf("agent.configure: param 'value' is required")
	}

	agentDir := filepath.Join(h.zlawHome, "agents", name)
	if err := config.WriteRuntimeFieldToDir(agentDir, key, value); err != nil {
		return fmt.Errorf("agent.configure: %w", err)
	}

	h.logger.Info("hub inbox: agent configured", "name", name, "key", key)
	return nil
}

// opAgentStop stops the named agent without restarting it.
func (h *ManagementHandler) opAgentStop(params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.stop: param 'name' is required")
	}
	return h.supervisor.Stop(name)
}

// opAgentRestart restarts the named agent.
func (h *ManagementHandler) opAgentRestart(params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.restart: param 'name' is required")
	}
	return h.supervisor.Restart(name)
}

// opAgentRemove stops the agent and removes it from the registry.
// Rejects if the target is the manager agent (self-protection).
func (h *ManagementHandler) opAgentRemove(params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.remove: param 'name' is required")
	}
	if name == h.managerName {
		return fmt.Errorf("agent.remove: cannot remove manager agent %q", name)
	}

	if err := h.supervisor.Stop(name); err != nil {
		// Log but don't fail if the agent wasn't running.
		h.logger.Warn("hub inbox: stop before remove failed", "name", name, "err", err)
	}
	h.registry.Deregister(name)
	h.logger.Info("hub inbox: agent removed", "name", name)
	return nil
}

// stringParam extracts a string value from params by key.
func stringParam(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
