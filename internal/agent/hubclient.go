package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/zsomething/zlaw/internal/identity"
	"github.com/zsomething/zlaw/internal/messaging"
)

const (
	// registrySubject is where agents publish their registration/heartbeat.
	registrySubject = "zlaw.registry"

	// inboxSubjectFmt is the agent-specific inbox subject pattern.
	inboxSubjectFmt = "agent.%s.inbox"

	// hubHeartbeatInterval is how often the agent re-publishes its registration.
	hubHeartbeatInterval = 30 * time.Second

	// agentInboxStream is the JetStream stream name for agent inbox messages.
	// Mirrors hub/internal/hub/stream.go constants.
	agentInboxStream = "AGENT_INBOX"
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
	Roles        []string `json:"roles"`
	PublicKey    string   `json:"public_key"`
}

// HubClient manages the agent's connection to the hub over NATS:
//   - publishes a registration/heartbeat to zlaw.registry on connect and every 30 s
//   - subscribes to agent.<id>.inbox for incoming task delegations
//   - runs each incoming task through HubTaskRunner and publishes a TaskReply
type HubClient struct {
	id           string
	version      string
	capabilities []string
	roles        []string
	seedPath     string // path to identity seed for signing
	messenger    messaging.Messenger
	runner       HubTaskRunner
	sysPromptFn  func() string
	logger       *slog.Logger
}

// NewHubClient creates a HubClient. sysPromptFn is called for each incoming
// task to get the current system prompt. seedPath is the path to the agent's
// identity keypair seed for signing registration messages; pass "" to skip signing.
func NewHubClient(
	id, version string,
	capabilities []string,
	roles []string,
	seedPath string,
	messenger messaging.Messenger,
	runner HubTaskRunner,
	sysPromptFn func() string,
	logger *slog.Logger,
) *HubClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &HubClient{
		id:           id,
		version:      version,
		capabilities: capabilities,
		roles:        roles,
		seedPath:     seedPath,
		messenger:    messenger,
		runner:       runner,
		sysPromptFn:  sysPromptFn,
		logger:       logger,
	}
}

// Start publishes the initial registration, begins the heartbeat loop, and
// subscribes to the agent's inbox. When JetStream is available it uses a durable
// pull consumer (redelivers on reconnect); otherwise it falls back to a plain
// nats.Subscribe. It returns when ctx is cancelled.
func (h *HubClient) Start(ctx context.Context) error {
	if err := h.publishRegistration(ctx); err != nil {
		return fmt.Errorf("hub: initial registration: %w", err)
	}

	inboxSubject := fmt.Sprintf(inboxSubjectFmt, h.id)

	// Try JetStream first; fall back to plain Subscribe.
	var err error
	if js := h.messenger.JetStream(); js != nil {
		err = h.runJetStreamInbox(ctx, inboxSubject, js)
	} else {
		err = h.runPlainInbox(ctx, inboxSubject)
	}
	return err
}

// runJetStreamInbox runs a durable pull consumer for the agent's inbox subject.
// Each call to Fetch() blocks until a message arrives or ctx is cancelled.
// Messages are acked after successful turn completion (redelivered on failure).
func (h *HubClient) runJetStreamInbox(ctx context.Context, inboxSubject string, js messaging.JetStreamer) error {
	consumer := h.id // durable consumer name == agent name

	if err := js.CreatePullConsumer(ctx, consumer, agentInboxStream, inboxSubject); err != nil {
		return fmt.Errorf("hub: create pull consumer %q: %w", consumer, err)
	}
	h.logger.Info("hub client started (jetstream)",
		"agent", h.id,
		"inbox", inboxSubject,
		"stream", agentInboxStream,
		"consumer", consumer,
	)

	// fetchTick controls how often the agent polls JetStream for new messages.
	// Keep it short enough to feel responsive but longer than the fetch timeout
	// so idle cycles don't spam logs with expected timeouts.
	fetchTick := time.NewTicker(2 * time.Second)
	defer fetchTick.Stop()
	ticker := time.NewTicker(hubHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("hub client stopping", "agent", h.id)
			return nil
		case <-ticker.C:
			if err := h.publishRegistration(ctx); err != nil {
				h.logger.Warn("hub: heartbeat failed", "agent", h.id, "err", err)
			}
		case <-fetchTick.C:
			// Short timeout: if no message, loop immediately. Routine timeouts
			// (DeadlineExceeded) are expected when the inbox is idle - don't log them.
			// Only log unexpected errors like network issues.
			fetchCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err := js.FetchOnSubject(fetchCtx, consumer, agentInboxStream, inboxSubject, func(msg *messaging.JetMsg) {
				h.processInboxMessage(ctx, msg.Data())
				_ = msg.Ack() // ack after successful processing
			})
			cancel()
			if err != nil && ctx.Err() == nil && !isRoutineTimeout(err) {
				h.logger.Debug("hub: fetch", "agent", h.id, "err", err)
			}
		}
	}
}

// runPlainInbox subscribes via plain nats.Subscribe and processes messages directly.
func (h *HubClient) runPlainInbox(ctx context.Context, inboxSubject string) error {
	sub, err := h.messenger.Subscribe(ctx, inboxSubject, func(data []byte) {
		h.processInboxMessage(ctx, data)
	})
	if err != nil {
		return fmt.Errorf("hub: subscribe to inbox %s: %w", inboxSubject, err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	h.logger.Info("hub client started",
		"agent", h.id,
		"inbox", inboxSubject,
		"capabilities", h.capabilities,
	)

	ticker := time.NewTicker(hubHeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("hub client stopping", "agent", h.id)
			return nil
		case <-ticker.C:
			if err := h.publishRegistration(ctx); err != nil {
				h.logger.Warn("hub: heartbeat failed", "agent", h.id, "err", err)
			}
		}
	}
}

// publishRegistration sends a registration message to zlaw.registry.
func (h *HubClient) publishRegistration(ctx context.Context) error {
	var publicKey string
	if h.seedPath != "" {
		pub, err := identity.ResolvePublicKey(h.seedPath)
		if err != nil {
			h.logger.Warn("hub: failed to resolve public key", "err", err)
		} else {
			publicKey = pub
		}
	}

	reg := hubRegistration{
		Name:         h.id,
		Version:      h.version,
		Capabilities: h.capabilities,
		Roles:        h.roles,
		PublicKey:    publicKey,
	}
	data, err := json.Marshal(reg)
	if err != nil {
		return err
	}
	return h.messenger.Publish(ctx, registrySubject, data)
}

// processInboxMessage parses a task envelope, runs the task through the runner,
// and publishes the reply to env.ReplyTo. It does not handle Ack/Nak.
func (h *HubClient) processInboxMessage(ctx context.Context, data []byte) {
	var env messaging.TaskEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		h.logger.Warn("hub: malformed task envelope", "agent", h.id, "err", err)
		return
	}
	if env.SessionID == "" || env.ReplyTo == "" || env.Task == "" {
		h.logger.Warn("hub: task envelope missing required fields",
			"agent", h.id,
			"session_id", env.SessionID,
			"reply_to", env.ReplyTo,
			"task_empty", env.Task == "",
		)
		return
	}

	h.logger.Info("hub: received task",
		"agent", h.id,
		"from", env.From,
		"session_id", env.SessionID,
		"reply_to", env.ReplyTo,
	)

	sysPrompt := ""
	if h.sysPromptFn != nil {
		sysPrompt = h.sysPromptFn()
	}

	output, runErr := h.runner.Run(ctx, env.SessionID, env.Task, sysPrompt)

	reply := messaging.TaskReply{SessionID: env.SessionID}
	if runErr != nil {
		reply.Error = runErr.Error()
		h.logger.Warn("hub: task run failed",
			"agent", h.id,
			"session_id", env.SessionID,
			"err", runErr,
		)
	} else {
		reply.Output = output
	}

	replyData, err := json.Marshal(reply)
	if err != nil {
		h.logger.Error("hub: marshal reply failed", "agent", h.id, "err", err)
		return
	}
	if err := h.messenger.Publish(ctx, env.ReplyTo, replyData); err != nil {
		h.logger.Error("hub: publish reply failed",
			"agent", h.id,
			"reply_to", env.ReplyTo,
			"err", err,
		)
	}
}

// isRoutineTimeout returns true if err is a routine timeout (no message available).
// These are expected when polling an idle inbox and should not be logged as errors.
func isRoutineTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
