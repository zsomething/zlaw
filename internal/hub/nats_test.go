package hub_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

func TestEnsureStoreDir(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "nats-store")

	got, err := hub.EnsureStoreDir(dir)
	if err != nil {
		t.Fatalf("EnsureStoreDir(%q): %v", dir, err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}

func TestEnsureStoreDir_Defaults(t *testing.T) {
	// EnsureStoreDir with empty string should return the default path.
	got, err := hub.EnsureStoreDir("")
	if err != nil {
		t.Fatalf("EnsureStoreDir(\"\"): %v", err)
	}
	want := hub.DefaultJetStreamStoreDir()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnsureStoreDir_CreatesNested(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "a", "b", "c")

	got, err := hub.EnsureStoreDir(dir)
	if err != nil {
		t.Fatalf("EnsureStoreDir(%q): %v", dir, err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("nested directory not created: %v", err)
	}
}

func TestStartNATS_JetStreamEnabled(t *testing.T) {
	tmp := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{
			Listen:   "127.0.0.1:14530",
			StoreDir: filepath.Join(tmp, "js-store"),
		},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer result.Conn.Close()

	if result.JetStream.StoreDir == "" {
		t.Error("expected StoreDir to be non-empty")
	}
}

func TestStartNATS_JetStreamDefaultStoreDir(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.HubConfig{
		NATS: config.NATSConfig{
			Listen: "127.0.0.1:14532",
			// StoreDir omitted — should default and be created.
		},
	}

	result, err := hub.StartNATS(ctx, cfg, "", slog.Default())
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer result.Conn.Close()

	wantDir := hub.DefaultJetStreamStoreDir()
	if result.JetStream.StoreDir != wantDir {
		t.Errorf("StoreDir = %q, want %q", result.JetStream.StoreDir, wantDir)
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Errorf("default store dir not created at %s: %v", wantDir, err)
	}
}
