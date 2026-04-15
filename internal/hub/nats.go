// Package hub implements the zlaw hub — the broker that routes messages between
// autonomous agent processes over an embedded NATS server.
package hub

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/zsomething/zlaw/internal/config"
)

// NATSResult holds the hub NATS connection and the generated ACL, which
// contains per-agent tokens to inject at spawn time.
type NATSResult struct {
	Conn      *nats.Conn
	ACL       *HubACL
	JetStream *JetStreamConfig // nil when JetStream is disabled
}

const (
	defaultNATSListen   = "127.0.0.1:4222"
	natsReadyTimeout    = 5 * time.Second
	defaultJetStreamDir = "nats"
)

// JetStreamConfig holds JetStream settings.
type JetStreamConfig struct {
	// Enabled activates the JetStream subsystem.
	Enabled bool
	// StoreDir is the directory for JetStream message storage.
	// Defaults to $ZLAW_HOME/nats when empty.
	StoreDir string
}

// DefaultJetStreamStoreDir returns the default JetStream store directory path.
func DefaultJetStreamStoreDir() string {
	return filepath.Join(config.ZlawHome(), defaultJetStreamDir)
}

// ControlSocketPath returns the path to the hub's control Unix socket.
func ControlSocketPath(zlawHome string) string {
	return filepath.Join(zlawHome, "run", controlSocketName)
}

// EnsureStoreDir creates the JetStream store directory if it does not exist.
// It returns the final (possibly updated) path.
func EnsureStoreDir(dir string) (string, error) {
	if dir == "" {
		dir = DefaultJetStreamStoreDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create jetstream store dir %s: %w", dir, err)
	}
	return dir, nil
}

// StartNATS starts an embedded NATS server using the hub config and returns a
// NATSResult containing the hub-internal connection and the generated ACL.
// The ACL carries per-agent tokens that the Supervisor injects at spawn time.
// The server is stopped when ctx is cancelled.
//
// If externalURL is non-empty the embedded server is not started; the function
// connects directly to that URL instead (ACL.AgentTokens will be empty and no
// auth is enforced in that case).
func StartNATS(ctx context.Context, cfg config.HubConfig, externalURL string, logger *slog.Logger) (*NATSResult, error) {
	if externalURL != "" {
		conn, err := connectExternal(ctx, externalURL, logger)
		if err != nil {
			return nil, err
		}
		return &NATSResult{Conn: conn, ACL: &HubACL{AgentTokens: make(AgentTokens)}}, nil
	}
	return startEmbedded(ctx, cfg, logger)
}

// startEmbedded starts the embedded nats-server with per-agent ACL and returns
// a NATSResult containing the hub connection and the generated ACL.
func startEmbedded(ctx context.Context, cfg config.HubConfig, logger *slog.Logger) (*NATSResult, error) {
	listen := cfg.NATS.Listen
	if listen == "" {
		listen = defaultNATSListen
	}

	host, port, err := parseHostPort(listen)
	if err != nil {
		return nil, fmt.Errorf("parse nats listen address %q: %w", listen, err)
	}

	acl, err := BuildHubACL(cfg.Agents)
	if err != nil {
		return nil, fmt.Errorf("build NATS ACL: %w", err)
	}

	var jsEnabled bool
	var jsStoreDir string

	// Configure JetStream if enabled in the hub config.
	if cfg.NATS.JetStream {
		jsStoreDir, err = EnsureStoreDir(cfg.NATS.StoreDir)
		if err != nil {
			return nil, fmt.Errorf("jetstream store dir: %w", err)
		}
		jsEnabled = true
		logger.Info("jetstream enabled", "store_dir", jsStoreDir)
	}

	var jsCfg *JetStreamConfig
	if jsEnabled {
		jsCfg = &JetStreamConfig{Enabled: true, StoreDir: jsStoreDir}
	}

	opts := &server.Options{
		Host:           host,
		Port:           port,
		NoLog:          true,
		NoSigs:         true,
		MaxControlLine: 4096,
		Users:          acl.Users,
		JetStream:      jsEnabled,
		StoreDir:       jsStoreDir,
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("create embedded nats server: %w", err)
	}

	go srv.Start()

	if !srv.ReadyForConnections(natsReadyTimeout) {
		srv.Shutdown()
		return nil, fmt.Errorf("embedded nats server did not become ready within %s", natsReadyTimeout)
	}

	logger.Info("embedded NATS server started", "listen", listen, "jetstream", jsEnabled)

	conn, err := nats.Connect(srv.ClientURL(), nats.UserInfo(hubUsername, acl.HubToken))
	if err != nil {
		srv.Shutdown()
		return nil, fmt.Errorf("connect to embedded nats: %w", err)
	}

	go func() {
		<-ctx.Done()
		conn.Drain() //nolint:errcheck
		srv.Shutdown()
		srv.WaitForShutdown()
		logger.Info("embedded NATS server stopped")
	}()

	return &NATSResult{Conn: conn, ACL: acl, JetStream: jsCfg}, nil
}

// connectExternal connects to an existing NATS server at url.
func connectExternal(ctx context.Context, url string, logger *slog.Logger) (*nats.Conn, error) {
	conn, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("connect to external nats %s: %w", url, err)
	}
	logger.Info("connected to external NATS server", "url", url)

	go func() {
		<-ctx.Done()
		conn.Drain() //nolint:errcheck
	}()

	return conn, nil
}

// parseHostPort splits a "host:port" listen string into host and port number.
func parseHostPort(listen string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}
