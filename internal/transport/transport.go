// Package transport abstracts the IPC mechanism used by the daemon and CLI attach.
package transport

import (
	"context"
	"net"
)

// Transport abstracts the listener and dialer used by the daemon and attach clients.
// The Phase 1 implementation is UnixTransport (Unix domain sockets).
// Future implementations (e.g. NamedPipeTransport for Windows, TCPTransport)
// can be swapped in behind this interface without changing daemon or attach code.
type Transport interface {
	// Listen creates a server-side listener. The caller is responsible for
	// closing the returned net.Listener.
	Listen(ctx context.Context) (net.Listener, error)
	// Dial connects to a server listening on this transport.
	Dial(ctx context.Context) (net.Conn, error)
	// Addr returns a human-readable address string for logging and display.
	Addr() string
}
