package hub

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/messaging"
)

// AuditEntry is the structured JSON record written to the audit log.
// Every field is captured at write time for forensic replay.
type AuditEntry struct {
	// Timestamp is the wall-clock time at write (RFC3339Nano).
	Timestamp string `json:"ts"`
	// TraceID propagates distributed trace across hops; empty on first hop.
	TraceID string `json:"trace_id,omitempty"`
	// From is the source agent ID (or "hub" for hub-initiated events).
	From string `json:"from"`
	// To is the destination agent ID (or "*" for broadcast).
	To string `json:"to"`
	// Subject is the NATS subject the message was on.
	Subject string `json:"subject"`
	// Type categorises the event.
	Type AuditEventType `json:"type"`
	// Direction is "in" or "out" relative to the hub.
	Direction string `json:"direction"`
	// SessionID is the conversation session (if applicable).
	SessionID string `json:"session_id,omitempty"`
	// Payload is the raw message bytes.
	Payload string `json:"payload,omitempty"`
	// Tool is the tool name for tool_call events.
	Tool string `json:"tool,omitempty"`
	// Error is set on error events.
	Error string `json:"error,omitempty"`
	// Duration is the round-trip time in milliseconds (for replies).
	DurationMs int64 `json:"duration_ms,omitempty"`
	// Extra holds additional event-specific metadata.
	Extra map[string]any `json:"extra,omitempty"`
}

// AuditEventType categorises audit log entries.
type AuditEventType string

const (
	// AuditEventRegister is logged when an agent publishes a registration/heartbeat.
	AuditEventRegister AuditEventType = "agent.register"
	// AuditEventTaskSent is logged when a TaskEnvelope is sent to an agent inbox.
	AuditEventTaskSent AuditEventType = "delegation.sent"
	// AuditEventTaskReply is logged when a TaskReply is received.
	AuditEventTaskReply AuditEventType = "delegation.reply"
	// AuditEventMgmt is logged for hub management API calls (agent.create, etc).
	AuditEventMgmt AuditEventType = "hub.management"
	// AuditEventDisconnect is logged when an agent's heartbeat expires.
	AuditEventDisconnect AuditEventType = "agent.disconnect"
	// AuditEventError is logged for delivery errors.
	AuditEventError AuditEventType = "error"
)

// AuditLogger subscribes to key NATS subjects and appends structured JSON
// entries to an append-only audit log file. It is safe for concurrent use
// by multiple goroutines.
type AuditLogger struct {
	logger *slog.Logger
	path   string
	f      *os.File
	mu     sync.Mutex
	conn   messaging.Messenger

	subscribers   []messaging.Subscription
	subjects      []string
	ctx           context.Context
	cancel        context.CancelFunc
	subscribersMu sync.Mutex // protects subscribers
}

// NewAuditLogger creates an AuditLogger that appends to auditPath.
// If auditPath is empty, audit logging is disabled (NewAuditLogger returns nil,
// no-op). The log file is created with append-only semantics.
func NewAuditLogger(auditPath string, conn messaging.Messenger, logger *slog.Logger) (*AuditLogger, error) {
	if auditPath == "" || conn == nil {
		return nil, nil // audit disabled
	}
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(auditPath), 0o700); err != nil {
		return nil, fmt.Errorf("audit: create dir: %w", err)
	}

	f, err := os.OpenFile(auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open file: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &AuditLogger{
		logger: logger,
		path:   auditPath,
		f:      f,
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// DefaultAuditSubjects are the NATS subjects the audit logger subscribes to.
var DefaultAuditSubjects = []string{
	"zlaw.registry",      // agent registration/heartbeats
	"zlaw.hub.inbox",     // hub management API calls
	"zlaw.registry.list", // registry query responses
}

// Start subscribes al to the audit subjects and begins appending entries.
// It returns when ctx is cancelled.
func (al *AuditLogger) Start() error {
	for _, subject := range al.subjects {
		sub, err := al.conn.Subscribe(al.ctx, subject, func(data []byte) {
			entry := al.buildEntry(subject, "in", data)
			al.writeEntry(entry)
		})
		if err != nil {
			return fmt.Errorf("audit: subscribe to %s: %w", subject, err)
		}
		al.subscribersMu.Lock()
		al.subscribers = append(al.subscribers, sub)
		al.subscribersMu.Unlock()
	}

	<-al.ctx.Done()
	return nil
}

// buildEntry parses a raw NATS message and returns a partially-populated
// AuditEntry. It extracts envelope fields where possible.
func (al *AuditLogger) buildEntry(subject, direction string, data []byte) AuditEntry {
	entry := AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Subject:   subject,
		Direction: direction,
		Payload:   string(data),
	}

	// Try to extract envelope fields based on subject.
	switch subject {
	case "zlaw.registry":
		// Registry messages: {"id":"...","version":"...","capabilities":[...]}
		var reg struct {
			ID           string   `json:"id"`
			Version      string   `json:"version"`
			Capabilities []string `json:"capabilities"`
		}
		if json.Unmarshal(data, &reg) == nil {
			entry.From = reg.ID
			entry.Extra = map[string]any{"version": reg.Version, "capabilities": reg.Capabilities}
		}
		entry.Type = AuditEventRegister

	case "zlaw.hub.inbox":
		// Hub management: delegation envelope or raw.
		var env struct {
			From      string `json:"from"`
			To        string `json:"to"`
			SessionID string `json:"session_id"`
			TraceID   string `json:"trace_id"`
			Type      string `json:"type"`
		}
		if json.Unmarshal(data, &env) == nil {
			entry.From = env.From
			entry.To = env.To
			entry.SessionID = env.SessionID
			entry.TraceID = env.TraceID
		}
		entry.Type = AuditEventMgmt

	case "zlaw.registry.list":
		entry.Direction = "out"
		entry.Type = AuditEventMgmt

	default:
		// Fallback: try generic envelope.
		var env struct {
			From      string `json:"from"`
			To        string `json:"to"`
			SessionID string `json:"session_id"`
			TraceID   string `json:"trace_id"`
		}
		if json.Unmarshal(data, &env) == nil {
			entry.From = env.From
			entry.To = env.To
			entry.SessionID = env.SessionID
			entry.TraceID = env.TraceID
			entry.Type = AuditEventTaskSent
		}
	}

	return entry
}

// writeEntry writes entry as a newline-delimited JSON record.
func (al *AuditLogger) writeEntry(entry AuditEntry) {
	al.mu.Lock()
	defer al.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		al.logger.Warn("audit: marshal failed", "err", err)
		return
	}
	data = append(data, '\n')
	if _, err := al.f.Write(data); err != nil {
		al.logger.Error("audit: write failed", "err", err)
	}
}

// LogEvent writes a manually constructed audit entry. Use for hub-initiated
// events (e.g., hub_start, hub_stop) that don't come from a NATS subject.
func (al *AuditLogger) LogEvent(entry AuditEntry) {
	if al == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if entry.From == "" {
		entry.From = "hub"
	}
	if entry.Direction == "" {
		entry.Direction = "out"
	}
	al.writeEntry(entry)
}

// Close closes the audit log file and unsubscribes all NATS subscriptions.
func (al *AuditLogger) Close() error {
	if al == nil {
		return nil
	}
	al.cancel()

	al.subscribersMu.Lock()
	for _, sub := range al.subscribers {
		sub.Unsubscribe() //nolint:errcheck
	}
	al.subscribers = nil
	al.subscribersMu.Unlock()

	return al.f.Close()
}

// SetSubjects replaces the list of subjects to audit. Call before Start.
func (al *AuditLogger) SetSubjects(subjects []string) {
	if al != nil {
		al.subjects = subjects
	}
}

// ReadEntries reads the last limit audit entries from the log file,
// optionally filtered by event type.
func (al *AuditLogger) ReadEntries(limit int, eventType string) ([]AuditEntry, error) {
	if al == nil || al.path == "" {
		return nil, nil
	}
	return ReadAuditLog(al.path, limit, eventType)
}

// Messenger implements messaging.Messenger for embedding in a chain.
// AuditLogger embeds a real Messenger (the NATS connection) and forwards
// Publish calls to it so it can be placed in a middleware chain.
// Subscribe is not used here — AuditLogger uses its own Start method instead.
func (al *AuditLogger) Publish(ctx context.Context, subject string, payload []byte) error {
	if al == nil || al.conn == nil {
		return nil
	}
	return al.conn.Publish(ctx, subject, payload)
}

func (al *AuditLogger) Subscribe(ctx context.Context, subject string, handler func([]byte)) (messaging.Subscription, error) {
	if al == nil || al.conn == nil {
		return nil, nil
	}
	return al.conn.Subscribe(ctx, subject, handler)
}

func (al *AuditLogger) Request(ctx context.Context, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	if al == nil || al.conn == nil {
		return nil, fmt.Errorf("audit logger: no connection")
	}
	return al.conn.Request(ctx, subject, payload, timeout)
}

func (al *AuditLogger) JetStream() messaging.JetStreamer {
	if al == nil || al.conn == nil {
		return nil
	}
	return al.conn.JetStream()
}

// ReadAuditLog reads the last limit entries from auditPath,
// optionally filtered by eventType. Entries are returned newest-last.
func ReadAuditLog(auditPath string, limit int, eventType string) ([]AuditEntry, error) {
	f, err := os.Open(auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var entries []AuditEntry
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}
		if eventType != "" && string(entry.Type) != eventType {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan audit log: %w", err)
	}

	// Trim to last limit entries.
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}
