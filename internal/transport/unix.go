package transport

import (
	"context"
	"fmt"
	"net"
	"os"
)

// UnixTransport implements Transport over a Unix domain socket.
// The socket file is removed before listening and after the listener closes.
type UnixTransport struct {
	path string
}

// NewUnixTransport returns a UnixTransport using the given socket path.
func NewUnixTransport(path string) *UnixTransport {
	return &UnixTransport{path: path}
}

// Listen creates a Unix socket listener at the configured path.
// Any pre-existing socket file is removed first to clear stale sockets from
// crashed daemon processes.
func (t *UnixTransport) Listen(_ context.Context) (net.Listener, error) {
	// Remove any stale socket file from a previous crash.
	if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("unix transport: remove stale socket %s: %w", t.path, err)
	}
	ln, err := net.Listen("unix", t.path)
	if err != nil {
		return nil, fmt.Errorf("unix transport: listen %s: %w", t.path, err)
	}
	return ln, nil
}

// Dial connects to a Unix socket at the configured path.
func (t *UnixTransport) Dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", t.path)
	if err != nil {
		return nil, fmt.Errorf("unix transport: dial %s: %w", t.path, err)
	}
	return conn, nil
}

// Addr returns the socket path.
func (t *UnixTransport) Addr() string {
	return t.path
}

// compile-time check.
var _ Transport = (*UnixTransport)(nil)
