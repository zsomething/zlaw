package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/zsomething/zlaw/internal/messaging"
)

const (
	// registrySubject is where agents publish their registration/heartbeat.
	registrySubject = "zlaw.registry"

	// inboxSubjectFmt is the agent-specific inbox subject pattern.
	inboxSubjectFmt = "agent.%s.inbox"

	// hubHeartbeatInterval is how often the agent re-publishes its registration.
	hubHeartbeatInterval = 30 * time.Second
)

// HubTaskRunner executes an incoming task envelope and returns the output text.
type HubTaskRunner interface {
	Run(ctx context.Context, sessionID, input, systemPrompt string) (string, error)
}

// hubRegistration is the JSON payload published to zlaw.registry.
type hubRegistration struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

// HubClient manages the agent's connection to the hub over NATS:
//   - publishes a registration/heartbeat to zlaw.registry on connect and every 30 s
//   - subscribes to agent.<name>.inbox for incoming task delegations
//   - runs each incoming task through HubTaskRunner and publishes a TaskReply
type HubClient struct {
	name         string
	version      string
	capabilities []string
	messenger    messaging.Messenger
	runner       HubTaskRunner
	sysPromptFn  func() string
	logger       *slog.Logger
}

// NewHubClient creates a HubClient. sysPromptFn is called for each incoming
// task to get the current system prompt.
func NewHubClient(
	name, version string,
	capabilities []string,
	messenger messaging.Messenger,
	runner HubTaskRunner,
	sysPromptFn func() string,
	logger *slog.Logger,
) *HubClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &HubClient{
		name:         name,
		version:      version,
		capabilities: capabilities,
		messenger:    messenger,
		runner:       runner,
		sysPromptFn:  sysPromptFn,
		logger:       logger,
	}
}

// Start publishes the initial registration, begins the heartbeat loop, and
// subscribes to the agent's inbox. It returns when ctx is cancelled.
func (h *HubClient) Start(ctx context.Context) error {
	if err := h.publishRegistration(ctx); err != nil {
		return fmt.Errorf("hub: initial registration: %w", err)
	}

	inboxSubject := fmt.Sprintf(inboxSubjectFmt, h.name)
	sub, err := h.messenger.Subscribe(ctx, inboxSubject, func(data []byte) {
		h.handleInbox(ctx, data)
	})
	if err != nil {
		return fmt.Errorf("hub: subscribe to inbox %s: %w", inboxSubject, err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	h.logger.Info("hub client started",
		"agent", h.name,
		"inbox", inboxSubject,
		"capabilities", h.capabilities,
	)

	ticker := time.NewTicker(hubHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("hub client stopping", "agent", h.name)
			return nil
		case <-ticker.C:
			if err := h.publishRegistration(ctx); err != nil {
				h.logger.Warn("hub: heartbeat failed", "agent", h.name, "err", err)
			}
		}
	}
}

// publishRegistration sends a registration message to zlaw.registry.
func (h *HubClient) publishRegistration(ctx context.Context) error {
	reg := hubRegistration{
		Name:         h.name,
		Version:      h.version,
		Capabilities: h.capabilities,
	}
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	return h.messenger.Publish(ctx, registrySubject, data)
}

// handleInbox processes a single message arriving on the agent's inbox subject.
func (h *HubClient) handleInbox(ctx context.Context, data []byte) {
	var env messaging.TaskEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		h.logger.Warn("hub: malformed task envelope", "agent", h.name, "err", err)
		return
	}
	if env.SessionID == "" || env.ReplyTo == "" {
		h.logger.Warn("hub: task envelope missing required fields",
			"agent", h.name,
			"session_id", env.SessionID,
			"reply_to", env.ReplyTo,
		)
		return
	}

	h.logger.Info("hub: received task",
		"agent", h.name,
		"session_id", env.SessionID,
		"reply_to", env.ReplyTo,
	)

	sysPrompt := ""
	if h.sysPromptFn != nil {
		sysPrompt = h.sysPromptFn()
	}

	output, runErr := h.runner.Run(ctx, env.SessionID, env.Input, sysPrompt)

	reply := messaging.TaskReply{SessionID: env.SessionID}
	if runErr != nil {
		reply.Error = runErr.Error()
		h.logger.Warn("hub: task run failed",
			"agent", h.name,
			"session_id", env.SessionID,
			"err", runErr,
		)
	} else {
		reply.Output = output
	}

	replyData, err := json.Marshal(reply)
	if err != nil {
		h.logger.Error("hub: marshal reply failed", "agent", h.name, "err", err)
		return
	}
	if err := h.messenger.Publish(ctx, env.ReplyTo, replyData); err != nil {
		h.logger.Error("hub: publish reply failed",
			"agent", h.name,
			"reply_to", env.ReplyTo,
			"err", err,
		)
	}
}
