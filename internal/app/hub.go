package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
	"github.com/zsomething/zlaw/internal/logging"
	"github.com/zsomething/zlaw/internal/messaging"
)

// StartHub loads the hub config and starts the hub process.
func StartHub(ctx context.Context, configPath string, externalNATSURL string, logger *slog.Logger, noColor bool) error {
	cfg, err := config.LoadHubConfig(configPath)
	if err != nil {
		return fmt.Errorf("load hub config: %w", err)
	}

	// Wrap logger with hub prefix and color.
	logger = setupHubLogger(logger, noColor)

	result, err := hub.StartNATS(ctx, cfg, externalNATSURL, logger)
	if err != nil {
		return fmt.Errorf("start nats: %w", err)
	}
	defer result.Conn.Close()

	// Create the durable agent inbox stream if JetStream is enabled.
	if result.JetStream != nil {
		sm := hub.NewStreamManager(result.Conn)
		if err := sm.EnsureAgentInboxStream(ctx, 0); err != nil {
			return fmt.Errorf("create agent inbox stream: %w", err)
		}
		logger.Info("agent inbox stream ready", "name", hub.AgentInboxStream)
	}

	selfBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	reg := hub.NewRegistry(logger)
	if err := reg.Start(ctx, result.Conn); err != nil {
		return fmt.Errorf("start registry: %w", err)
	}

	// Create a messenger from the NATS connection for log publishing.
	messenger := &natsMessengerAdapter{conn: result.Conn}

	sup := hub.NewSupervisorWithMessenger(cfg, result.Conn.ConnectedUrl(), selfBin, "", result.ACL.AgentTokens, logger, noColor, messenger)
	if err := sup.Start(ctx); err != nil {
		return fmt.Errorf("start supervisor: %w", err)
	}

	managerName := managerAgentName(cfg)
	mgmtHandler := hub.NewManagementHandler(
		result.Conn,
		sup,
		reg,
		managerName,
		config.ZlawHome(),
		logger,
	)
	go func() {
		if err := mgmtHandler.Start(ctx); err != nil && ctx.Err() == nil {
			logger.Error("hub management handler stopped unexpectedly", "err", err)
		}
	}()

	logger.Info("hub started",
		"name", cfg.Hub.Name,
		"agents", len(cfg.Agents),
		"manager", managerName,
	)

	// Block until context is cancelled (signal or parent shutdown).
	<-ctx.Done()
	logger.Info("hub shutting down")
	return nil
}

// managerAgentName returns the name of the first agent entry marked Manager,
// or empty string if none.
func managerAgentName(cfg config.HubConfig) string {
	for _, a := range cfg.Agents {
		if a.Manager {
			return a.Name
		}
	}
	return ""
}

// HubStatus prints the current hub status.
// Phase 2 stub — will query the supervisor via Unix socket.
func HubStatus() error {
	fmt.Println("Hub status: not running")
	fmt.Println("(Phase 2 will query the supervisor process via Unix socket)")
	return nil
}

// setupHubLogger wraps the logger with hub prefix and color.
func setupHubLogger(logger *slog.Logger, noColor bool) *slog.Logger {
	opts := logging.Options{
		Label:   "[hub]",
		Color:   logging.ColorGray,
		NoColor: noColor,
		Time:    logging.DetectTimeFormat(),
	}
	h := logging.NewPrettyHandler(os.Stderr, opts)
	return slog.New(h)
}

// natsMessengerAdapter adapts *nats.Conn to messaging.Messenger interface.
type natsMessengerAdapter struct {
	conn *nats.Conn
}

func (a *natsMessengerAdapter) Publish(_ context.Context, subject string, payload []byte) error {
	return a.conn.Publish(subject, payload)
}

func (a *natsMessengerAdapter) Subscribe(_ context.Context, subject string, handler func([]byte)) (messaging.Subscription, error) {
	sub, err := a.conn.Subscribe(subject, func(msg *nats.Msg) {
		handler(msg.Data)
	})
	if err != nil {
		return nil, err
	}
	return &natsSubscription{sub: sub}, nil
}

func (a *natsMessengerAdapter) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	msg, err := a.conn.RequestWithContext(ctx, subject, payload)
	if err != nil {
		return nil, err
	}
	return msg.Data, nil
}

func (a *natsMessengerAdapter) JetStream() messaging.JetStreamer {
	return nil // Hub doesn't need JetStream for log publishing
}

type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

var _ messaging.Messenger = (*natsMessengerAdapter)(nil)
