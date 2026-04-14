// Package daemon implements the server-side of the daemon transport.
// It listens on a Transport, accepts CLI attach connections, and routes
// user_turn events to the session manager.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/session"
	"github.com/zsomething/zlaw/internal/transport"
)

// Daemon listens on a Transport and manages CLI attach connections.
type Daemon struct {
	transport transport.Transport
	manager   *session.Manager
	pidFile   string
	logger    *slog.Logger
}

// New creates a Daemon.
func New(t transport.Transport, m *session.Manager, pidFile string, logger *slog.Logger) *Daemon {
	return &Daemon{
		transport: t,
		manager:   m,
		pidFile:   pidFile,
		logger:    logger,
	}
}

// Serve writes a PID file, listens for incoming CLI attach connections, and
// blocks until ctx is cancelled. After the listener closes it drains in-flight
// turns and broadcasts EventShutdown to all connected sinks before returning.
// drainTimeout controls how long it waits; 0 uses a default of 60 s.
func (d *Daemon) Serve(ctx context.Context, drainTimeout time.Duration) error {
	if drainTimeout <= 0 {
		drainTimeout = 60 * time.Second
	}
	if d.pidFile != "" {
		if err := os.MkdirAll(filepath.Dir(d.pidFile), 0o755); err != nil {
			return fmt.Errorf("daemon: create pid dir: %w", err)
		}
		pid := strconv.Itoa(os.Getpid())
		if err := os.WriteFile(d.pidFile, []byte(pid+"\n"), 0o644); err != nil {
			return fmt.Errorf("daemon: write pid file: %w", err)
		}
		defer os.Remove(d.pidFile) //nolint:errcheck
		d.logger.Info("daemon: pid file written", "path", d.pidFile, "pid", pid)
	}

	ln, err := d.transport.Listen(ctx)
	if err != nil {
		return fmt.Errorf("daemon: listen: %w", err)
	}
	defer ln.Close() //nolint:errcheck

	d.logger.Info("daemon: listening", "address", d.transport.Addr())

	// Close the listener when ctx is cancelled so ln.Accept() unblocks.
	go func() {
		<-ctx.Done()
		ln.Close() //nolint:errcheck
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // ctx cancelled — fall through to graceful drain
			}
			d.logger.Error("daemon: accept error", "error", err)
			continue
		}
		go d.handleConn(ctx, conn)
	}

	// Graceful drain: notify attached clients and wait for in-flight turns.
	d.logger.Info("daemon: draining in-flight turns", "timeout", drainTimeout)
	drainCtx, cancel := context.WithTimeout(context.Background(), drainTimeout) //nolint:contextcheck // intentionally independent from cancelled ctx
	defer cancel()
	d.manager.BroadcastAll(drainCtx, session.Event{Type: session.EventShutdown}) //nolint:contextcheck
	d.manager.Drain(drainCtx)                                                    //nolint:contextcheck
	if drainCtx.Err() != nil {
		d.logger.Warn("daemon: drain timeout exceeded, forcing shutdown")
	} else {
		d.logger.Info("daemon: shutdown complete")
	}
	return nil
}

// handleConn manages a single CLI attach connection.
// Protocol:
//  1. Read first event; must be EventSubscribe with a non-empty SessionID.
//  2. Add a connSink to the session's broadcaster.
//  3. Read subsequent events; EventUserTurn is routed to the session manager.
//  4. On connection close or ctx cancellation, remove the sink.
func (d *Daemon) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close() //nolint:errcheck

	// Close conn when ctx is cancelled so blocking reads unblock.
	go func() {
		<-ctx.Done()
		conn.Close() //nolint:errcheck
	}()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	// First message must be a subscribe event.
	var sub session.Event
	if err := dec.Decode(&sub); err != nil {
		d.logger.Warn("daemon: failed to read subscribe message", "error", err)
		return
	}
	if sub.Type != session.EventSubscribe {
		d.logger.Warn("daemon: expected subscribe event", "got", sub.Type)
		return
	}
	if sub.SessionID == "" {
		d.logger.Warn("daemon: subscribe event missing session_id")
		return
	}

	sessionID := sub.SessionID
	s := d.manager.GetOrCreate(ctx, sessionID)
	sink := newConnSink(conn, enc)
	s.Broadcaster.Add(sink)
	defer func() {
		s.Broadcaster.Remove(sink)
		sink.Close() //nolint:errcheck
	}()

	d.logger.Info("daemon: client attached", "session_id", sessionID)

	for {
		var e session.Event
		if err := dec.Decode(&e); err != nil {
			if ctx.Err() != nil {
				return
			}
			d.logger.Info("daemon: client disconnected", "session_id", sessionID)
			return
		}
		if e.Type == session.EventUserTurn && e.Data != "" {
			d.manager.Submit(ctx, sessionID, e.Data, "cli-attach")
		}
	}
}

// connSink is an OutputSink that writes events as newline-delimited JSON to a
// net.Conn. It is used by CLI attach connections.
type connSink struct {
	conn net.Conn
	enc  *json.Encoder
	mu   sync.Mutex
}

func newConnSink(conn net.Conn, enc *json.Encoder) *connSink {
	return &connSink{conn: conn, enc: enc}
}

func (s *connSink) Capabilities() session.ChannelCaps {
	return session.ChannelCaps{Streaming: true, TypingIndicator: true}
}

// SendTyping sends an EventThinking event, which the attach client renders
// as a progress indicator.
func (s *connSink) SendTyping(ctx context.Context) error {
	return s.Send(ctx, session.Event{Type: session.EventThinking})
}

func (s *connSink) Send(_ context.Context, e session.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(e) // json.Encoder.Encode always appends a newline
}

func (s *connSink) Close() error {
	return s.conn.Close()
}

// compile-time check.
var _ session.OutputSink = (*connSink)(nil)
