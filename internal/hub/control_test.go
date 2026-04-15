package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/config"
)

type mockControlSupervisor struct {
	statuses []AgentStatus
	status   map[string]AgentStatus
}

func (m *mockControlSupervisor) Statuses() []AgentStatus { return m.statuses }
func (m *mockControlSupervisor) Status(name string) (AgentStatus, error) {
	if s, ok := m.status[name]; ok {
		return s, nil
	}
	return AgentStatus{}, nil
}
func (m *mockControlSupervisor) Stop(name string) error                             { return nil }
func (m *mockControlSupervisor) Restart(name string) error                          { return nil }
func (m *mockControlSupervisor) Spawn(_ context.Context, _ config.AgentEntry) error { return nil }

type mockControlRegistry struct {
	entries []RegistryEntry
}

func (m *mockControlRegistry) List() []RegistryEntry  { return m.entries }
func (m *mockControlRegistry) Deregister(name string) {}

func TestControlSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	sup := &mockControlSupervisor{
		statuses: []AgentStatus{
			{Name: "alice", Running: true, PID: 1234},
			{Name: "bob", Running: false, PID: 0},
		},
		status: map[string]AgentStatus{
			"alice": {Name: "alice", Running: true, PID: 1234},
			"bob":   {Name: "bob", Running: false, PID: 0},
		},
	}
	reg := &mockControlRegistry{
		entries: []RegistryEntry{
			{Name: "alice", Status: AgentConnected},
			{Name: "bob", Status: AgentDisconnected},
		},
	}

	ctrl := NewControlSocket(sockPath, sup, reg, nil, config.HubConfig{
		Hub: config.HubMeta{Name: "test-hub"},
	}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer ctrl.Stop() //nolint:errcheck

	// Small delay for socket to be ready.
	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Helper to send a request and get the result.
	send := func(method string) (json.RawMessage, error) {
		req := map[string]any{"method": method}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, err := conn.Write(append(data, '\n'))
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		return raw, nil
	}

	t.Run("hub.status", func(t *testing.T) {
		raw, err := send("hub.status")
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		var resp struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
		var result struct {
			Name       string `json:"name"`
			AgentCount int    `json:"agent_count"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if result.Name != "test-hub" {
			t.Errorf("Name = %q, want %q", result.Name, "test-hub")
		}
		if result.AgentCount != 2 {
			t.Errorf("AgentCount = %d, want 2", result.AgentCount)
		}
	})

	t.Run("agent.list", func(t *testing.T) {
		raw, err := send("agent.list")
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		var resp struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok")
		}
		var result struct {
			Agents []RegistryEntry `json:"agents"`
		}
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if len(result.Agents) != 2 {
			t.Fatalf("Agents len = %d, want 2", len(result.Agents))
		}
	})

	t.Run("agent.status", func(t *testing.T) {
		req := map[string]any{"method": "agent.status", "params": map[string]any{"name": "alice"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("send: %v", err)
		}
		var resp struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok")
		}
		var result AgentStatus
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if result.Name != "alice" {
			t.Errorf("Name = %q, want %q", result.Name, "alice")
		}
		if !result.Running {
			t.Error("Running = false, want true")
		}
		if result.PID != 1234 {
			t.Errorf("PID = %d, want 1234", result.PID)
		}
	})

	t.Run("unknown method", func(t *testing.T) {
		raw, err := send("unknown.method")
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.OK {
			t.Fatal("expected not ok")
		}
	})
}
