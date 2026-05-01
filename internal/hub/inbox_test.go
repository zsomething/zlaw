package hub_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

// --- stubs ---

type stubSpawner struct {
	spawned   []string
	stopped   []string
	restarted []string
}

func (s *stubSpawner) Spawn(_ context.Context, entry config.AgentEntry) error {
	s.spawned = append(s.spawned, entry.ID)
	return nil
}

func (s *stubSpawner) Stop(name string) error {
	s.stopped = append(s.stopped, name)
	return nil
}

func (s *stubSpawner) Restart(name string) error {
	s.restarted = append(s.restarted, name)
	return nil
}

type stubRegistry struct {
	entries map[string]hub.RegistryEntry
}

func (r *stubRegistry) List() []hub.RegistryEntry {
	result := make([]hub.RegistryEntry, 0, len(r.entries))
	for _, e := range r.entries {
		result = append(result, e)
	}
	return result
}
func (r *stubRegistry) Get(name string) (hub.RegistryEntry, bool) {
	e, ok := r.entries[name]
	return e, ok
}
func (r *stubRegistry) Deregister(name string) {
	delete(r.entries, name)
}

// --- helpers ---

// runInbox starts a ManagementHandler on the given nc, returns a request helper.
func runInbox(t *testing.T, sup hub.AgentSpawner, reg hub.AgentRegistryReader, zlawHome string) func(hub.ManagementRequest) hub.ManagementReply {
	t.Helper()
	nc := startTestNATS(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	h := hub.NewManagementHandler(nc, sup, reg, zlawHome, slog.Default())
	go func() { _ = h.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	return func(req hub.ManagementRequest) hub.ManagementReply {
		t.Helper()
		data, _ := json.Marshal(req)
		msg, err := nc.Request("zlaw.hub.inbox", data, 2*time.Second)
		if err != nil {
			t.Fatalf("request op=%s: %v", req.Op, err)
		}
		var reply hub.ManagementReply
		if err := json.Unmarshal(msg.Data, &reply); err != nil {
			t.Fatalf("unmarshal op=%s: %v", req.Op, err)
		}
		return reply
	}
}

// --- tests ---

func TestManagementHandler_AgentList(t *testing.T) {
	reg := &stubRegistry{entries: map[string]hub.RegistryEntry{
		"worker":  {ID: "worker", Status: hub.AgentConnected},
		"analyst": {ID: "analyst", Status: hub.AgentConnected},
	}}
	req := runInbox(t, &stubSpawner{}, reg, t.TempDir())

	reply := req(hub.ManagementRequest{Op: "agent.list"})
	if !reply.OK {
		t.Fatalf("expected OK, got error: %s", reply.Error)
	}
	if reply.Result == nil {
		t.Error("expected non-nil result")
	}
}

func TestManagementHandler_AgentCreate(t *testing.T) {
	zlawHome := t.TempDir()
	sup := &stubSpawner{}
	req := runInbox(t, sup, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, zlawHome)

	// Hub no longer scaffolds files — ctl creates the agent dir first.
	agentDir := filepath.Join(zlawHome, "agents", "newbot")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatalf("mkdir agentDir: %v", err)
	}
	// Pre-create zlaw.toml so AddAgent can write to it.
	zlawTOML := filepath.Join(zlawHome, "zlaw.toml")
	if err := os.WriteFile(zlawTOML, []byte("[hub]\nname=\"test\"\n"), 0o600); err != nil {
		t.Fatalf("write zlaw.toml: %v", err)
	}
	t.Setenv("ZLAW_HOME", zlawHome)

	reply := req(hub.ManagementRequest{
		Op:     "agent.create",
		Params: map[string]any{"id": "newbot", "dir": agentDir},
	})
	if !reply.OK {
		t.Fatalf("expected OK, got error: %s", reply.Error)
	}
	if len(sup.spawned) != 1 || sup.spawned[0] != "newbot" {
		t.Errorf("expected newbot spawned, got: %v", sup.spawned)
	}
}

func TestManagementHandler_AgentCreate_MissingName(t *testing.T) {
	req := runInbox(t, &stubSpawner{}, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, t.TempDir())

	reply := req(hub.ManagementRequest{
		Op:     "agent.create",
		Params: map[string]any{},
	})
	if reply.OK {
		t.Error("expected error when name is missing")
	}
}

func TestManagementHandler_AgentConfigure(t *testing.T) {
	zlawHome := t.TempDir()
	agentDir := filepath.Join(zlawHome, "agents", "worker")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}

	req := runInbox(t, &stubSpawner{}, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, zlawHome)

	reply := req(hub.ManagementRequest{
		Op: "agent.configure",
		Params: map[string]any{
			"id":    "worker",
			"key":   "llm.model",
			"value": "claude-opus-4-6",
		},
	})
	if !reply.OK {
		t.Fatalf("expected OK, got error: %s", reply.Error)
	}

	rtPath := filepath.Join(agentDir, "runtime.toml")
	if _, err := os.Stat(rtPath); err != nil {
		t.Errorf("runtime.toml not written at %s: %v", rtPath, err)
	}
}

func TestManagementHandler_AgentConfigure_InvalidKey(t *testing.T) {
	zlawHome := t.TempDir()
	agentDir := filepath.Join(zlawHome, "agents", "worker")
	os.MkdirAll(agentDir, 0o700) //nolint:errcheck

	req := runInbox(t, &stubSpawner{}, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, zlawHome)

	reply := req(hub.ManagementRequest{
		Op: "agent.configure",
		Params: map[string]any{
			"id":    "worker",
			"key":   "llm.backend", // not in allowlist
			"value": "openai",
		},
	})
	if reply.OK {
		t.Error("expected error for non-allowlisted key")
	}
}

func TestManagementHandler_AgentStop(t *testing.T) {
	sup := &stubSpawner{}
	req := runInbox(t, sup, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, t.TempDir())

	reply := req(hub.ManagementRequest{
		Op:     "agent.stop",
		Params: map[string]any{"id": "worker"},
	})
	if !reply.OK {
		t.Fatalf("expected OK, got: %s", reply.Error)
	}
	if len(sup.stopped) != 1 || sup.stopped[0] != "worker" {
		t.Errorf("expected worker stopped, got: %v", sup.stopped)
	}
}

func TestManagementHandler_AgentRestart(t *testing.T) {
	sup := &stubSpawner{}
	req := runInbox(t, sup, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, t.TempDir())

	reply := req(hub.ManagementRequest{
		Op:     "agent.restart",
		Params: map[string]any{"id": "worker"},
	})
	if !reply.OK {
		t.Fatalf("expected OK, got: %s", reply.Error)
	}
	if len(sup.restarted) != 1 || sup.restarted[0] != "worker" {
		t.Errorf("expected worker restarted, got: %v", sup.restarted)
	}
}

func TestManagementHandler_AgentRemove(t *testing.T) {
	sup := &stubSpawner{}
	reg := &stubRegistry{entries: map[string]hub.RegistryEntry{"worker": {ID: "worker"}}}
	req := runInbox(t, sup, reg, t.TempDir())

	reply := req(hub.ManagementRequest{
		Op:     "agent.remove",
		Params: map[string]any{"id": "worker"},
	})
	if !reply.OK {
		t.Fatalf("expected OK, got: %s", reply.Error)
	}
	for _, e := range reg.List() {
		if e.ID == "worker" {
			t.Error("expected worker deregistered")
		}
	}
}

func TestManagementHandler_AgentRemove_NoSelfProtection(t *testing.T) {
	// In the P2P model (#273), no agent has special protection.
	// Any agent can be removed.
	req := runInbox(t, &stubSpawner{}, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, t.TempDir())

	reply := req(hub.ManagementRequest{
		Op:     "agent.remove",
		Params: map[string]any{"id": "any-agent"},
	})
	// No error — any agent can be removed in the P2P model.
	if !reply.OK {
		t.Errorf("expected OK for agent.remove in P2P model; got: %s", reply.Error)
	}
}

func TestManagementHandler_UnknownOp(t *testing.T) {
	req := runInbox(t, &stubSpawner{}, &stubRegistry{entries: map[string]hub.RegistryEntry{}}, t.TempDir())

	reply := req(hub.ManagementRequest{Op: "no.such.op"})
	if reply.OK {
		t.Error("expected error for unknown op")
	}
}
