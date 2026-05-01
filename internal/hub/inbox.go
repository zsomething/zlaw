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
backend = "anthropic"
model = "claude-sonnet-4-5"
auth_profile = "anthropic"
max_tokens = 4096
timeout_sec = 60

[tools]
allowed = []

# Uncomment and configure adapters after setting up credentials:
# [[adapter]]
# type = "telegram"
# auth_profile = "telegram"
`

	// scaffoldCredentialsTOML is the initial credentials file for a new agent.
	// Values use ${ENV_VAR} expansion so the hub can inject real values.
	scaffoldCredentialsTOML = `[profiles.anthropic]
name = "anthropic"
data = { api_key = "${ANTHROPIC_API_KEY}" }

[profiles.telegram]
name = "telegram"
data = { telegram_bot_token = "${TELEGRAM_BOT_TOKEN}" }

[profiles.fizzy]
name = "fizzy"
data = { fizzy_api_key = "${FIZZY_API_KEY}" }
`

	// scaffoldSoulMD is the default SOUL.md for a new workspace.
	scaffoldSoulMD = `You are a helpful personal assistant.
`

	// scaffoldIdentityMD is the default IDENTITY.md for a new workspace.
	scaffoldIdentityMD = `# Identity

Your name is %s.
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
	Get(name string) (RegistryEntry, bool)
	Deregister(name string)
}

// ManagementHandler subscribes to zlaw.hub.inbox and dispatches management
// operations (agent.list, agent.create, agent.configure, agent.stop,
// agent.restart, agent.remove) to the Supervisor and Registry.
//
// Note: hub management API is CLI-only (#273). The NATS zlaw.hub.inbox
// handler is retained for possible future use but the hub's control socket
// is the primary interface for lifecycle operations.
type ManagementHandler struct {
	conn       *nats.Conn
	supervisor AgentSpawner
	registry   AgentRegistryReader
	zlawHome   string
	logger     *slog.Logger
}

// NewManagementHandler creates a ManagementHandler. It receives a supervisor
// and registry to dispatch agent lifecycle operations.
func NewManagementHandler(
	conn *nats.Conn,
	supervisor AgentSpawner,
	registry AgentRegistryReader,
	zlawHome string,
	logger *slog.Logger,
) *ManagementHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ManagementHandler{
		conn:       conn,
		supervisor: supervisor,
		registry:   registry,
		zlawHome:   zlawHome,
		logger:     logger,
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

// AgentCreate scaffolds a new agent directory and spawns the agent.
// It is the public equivalent of opAgentCreate used by the control socket.
func (h *ManagementHandler) AgentCreate(ctx context.Context, name string, workspaceDir string) error {
	return h.opAgentCreate(ctx, map[string]any{
		"name":      name,
		"workspace": workspaceDir,
	})
}

// opAgentCreate scaffolds a new agent directory and spawns the agent.
func (h *ManagementHandler) opAgentCreate(ctx context.Context, params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.create: param 'name' is required")
	}

	agentDir := filepath.Join(h.zlawHome, "agents", name)
	ws, _ := stringParam(params, "workspace")
	if ws == "" {
		ws = filepath.Join(h.zlawHome, "workspaces", name)
	}
	workspaceDir := ws

	// Create agent dir and workspace dir.
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return fmt.Errorf("agent.create: create agent dir %s: %w", agentDir, err)
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("agent.create: create workspace dir %s: %w", workspaceDir, err)
	}

	// Scaffold agent.toml if not exists.
	agentTOMLPath := filepath.Join(agentDir, "agent.toml")
	if _, err := os.Stat(agentTOMLPath); os.IsNotExist(err) {
		content := fmt.Sprintf(scaffoldAgentTOML, name)
		if err := os.WriteFile(agentTOMLPath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("agent.create: write agent.toml: %w", err)
		}
	}

	// Scaffold per-agent credentials.toml if not exists.
	credsPath := filepath.Join(agentDir, "credentials.toml")
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		if err := os.WriteFile(credsPath, []byte(scaffoldCredentialsTOML), 0o600); err != nil {
			return fmt.Errorf("agent.create: write credentials.toml: %w", err)
		}
	}

	// Scaffold workspace files if not exist.
	soulPath := filepath.Join(workspaceDir, "SOUL.md")
	if _, err := os.Stat(soulPath); os.IsNotExist(err) {
		if err := os.WriteFile(soulPath, []byte(scaffoldSoulMD), 0o644); err != nil {
			return fmt.Errorf("agent.create: write SOUL.md: %w", err)
		}
	}
	identityPath := filepath.Join(workspaceDir, "IDENTITY.md")
	if _, err := os.Stat(identityPath); os.IsNotExist(err) {
		content := fmt.Sprintf(scaffoldIdentityMD, name)
		if err := os.WriteFile(identityPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("agent.create: write IDENTITY.md: %w", err)
		}
	}

	entry := config.AgentEntry{
		Name:          name,
		Dir:           agentDir,
		RestartPolicy: config.RestartOnFailure,
	}

	if err := h.supervisor.Spawn(ctx, entry); err != nil {
		return fmt.Errorf("agent.create: spawn: %w", err)
	}

	// Persist the agent to zlaw.toml so it survives hub restarts.
	var cfg config.HubConfig
	if err := cfg.AddAgent(entry); err != nil {
		h.logger.Warn("hub inbox: agent spawned but not persisted to zlaw.toml", "name", name, "err", err)
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
// No special protection for any agent — hub management is CLI-only (#273).
func (h *ManagementHandler) opAgentRemove(params map[string]any) error {
	name, ok := stringParam(params, "name")
	if !ok || name == "" {
		return fmt.Errorf("agent.remove: param 'name' is required")
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

// toolSubject is the NATS subject for hub tool calls.
const toolSubject = "zlaw.hub.tool"

// StartToolInbox subscribes to zlaw.hub.tool and dispatches incoming
// tool-call requests to handler. It blocks until ctx is cancelled.
func (h *ManagementHandler) StartToolInbox(ctx context.Context, handler *HubInbox) error {
	sub, err := h.conn.Subscribe(toolSubject, func(msg *nats.Msg) {
		var req ToolRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			h.logger.Warn("hub tool inbox: malformed tool request", "err", err)
			return
		}
		if req.Tool == "" {
			h.logger.Warn("hub tool inbox: request missing tool name")
			return
		}

		reply := handler.HandleToolRequest(ctx, req)
		replyData, err := json.Marshal(reply)
		if err != nil {
			h.logger.Error("hub tool inbox: marshal reply failed", "err", err)
			return
		}

		// Publish reply to the caller-provided inbox subject.
		if req.ReplyTo != "" {
			if pubErr := h.conn.Publish(req.ReplyTo, replyData); pubErr != nil {
				h.logger.Error("hub tool inbox: publish reply failed",
					"reply_to", req.ReplyTo, "err", pubErr)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("hub tool inbox: subscribe to %s: %w", toolSubject, err)
	}

	h.logger.Info("hub tool inbox active", "subject", toolSubject)

	<-ctx.Done()
	sub.Unsubscribe() //nolint:errcheck
	return nil
}

// HandleTool implements ToolHandler. It dispatches tool calls via the
// existing dispatch logic, using the tool name as the op.
func (h *ManagementHandler) HandleTool(ctx context.Context, req ToolRequest) ToolReply {
	params := req.Args // may be nil
	result, err := h.dispatch(ctx, req.Tool, params)
	if err != nil {
		return ToolReply{Tool: req.Tool, OK: false, Error: err.Error()}
	}
	data, _ := json.Marshal(result)
	return ToolReply{Tool: req.Tool, OK: true, Output: string(data)}
}
