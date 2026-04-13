package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/zsomething/zlaw/internal/session"
	"github.com/zsomething/zlaw/internal/transport"
)

// Attach connects to a running daemon, subscribes to a session, and starts
// a bidirectional event loop:
//   - Events from the daemon are rendered to stdout.
//   - Lines read from stdin are sent to the daemon as user_turn events.
//
// Detaching (Ctrl-C or EOF on stdin) closes the connection without ending
// the session on the daemon side.
func Attach(ctx context.Context, t transport.Transport, sessionID string, logger *slog.Logger) error {
	conn, err := t.Dial(ctx)
	if err != nil {
		return fmt.Errorf("attach: dial %s: %w", t.Addr(), err)
	}
	defer conn.Close() //nolint:errcheck

	// Close the connection when ctx is cancelled so blocking reads unblock.
	go func() {
		<-ctx.Done()
		conn.Close() //nolint:errcheck
	}()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	// Register interest in the session.
	if err := enc.Encode(session.Event{
		Type:      session.EventSubscribe,
		SessionID: sessionID,
	}); err != nil {
		return fmt.Errorf("attach: subscribe: %w", err)
	}

	// Start a goroutine that forwards stdin lines as user_turn events.
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Fprint(os.Stdout, "> ")
			if !scanner.Scan() {
				return
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if err := enc.Encode(session.Event{
				Type:      session.EventUserTurn,
				SessionID: sessionID,
				Data:      line,
			}); err != nil {
				logger.Error("attach: send user_turn", "error", err)
				return
			}
		}
	}()

	// Read and render events from the daemon.
	for {
		var e session.Event
		if err := dec.Decode(&e); err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("attach: read event: %w", err)
		}
		renderAttachEvent(e)
	}
}

// renderAttachEvent prints a human-readable representation of a daemon event.
func renderAttachEvent(e session.Event) {
	switch e.Type {
	case session.EventAssistantDelta:
		fmt.Print(e.Data)
	case session.EventAssistantDone:
		// Ensure the streamed response ends with a newline.
		if e.Data != "" && !strings.HasSuffix(e.Data, "\n") {
			fmt.Println()
		} else {
			fmt.Println()
		}
	case session.EventThinking:
		fmt.Print(".")
	case session.EventToolCall:
		fmt.Fprintf(os.Stdout, "\n[tool: %s]\n", e.Data)
	case session.EventError:
		fmt.Fprintf(os.Stderr, "\nerror: %s\n", e.Data)
	case session.EventShutdown:
		fmt.Fprintln(os.Stdout, "\n[daemon shutting down]")
	}
}
