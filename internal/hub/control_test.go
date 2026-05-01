package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/config"
)

type mockControlSupervisor struct {
	statuses []AgentStatus
	status   map[string]AgentStatus
	// Track lifecycle calls for assertions.
	SpawnedNames   []string
	RemovedNames   []string
	StoppedNames   []string
	RestartedNames []string
}

func (m *mockControlSupervisor) Statuses() []AgentStatus { return m.statuses }
func (m *mockControlSupervisor) Status(name string) (AgentStatus, error) {
	if s, ok := m.status[name]; ok {
		return s, nil
	}
	return AgentStatus{}, nil
}
func (m *mockControlSupervisor) Stop(name string) error {
	m.StoppedNames = append(m.StoppedNames, name)
	return nil
}
func (m *mockControlSupervisor) Restart(name string) error {
	m.RestartedNames = append(m.RestartedNames, name)
	return nil
}
func (m *mockControlSupervisor) Spawn(_ context.Context, entry config.AgentEntry) error {
	m.SpawnedNames = append(m.SpawnedNames, entry.Name)
	return nil
}
func (m *mockControlSupervisor) Remove(name string) error {
	m.RemovedNames = append(m.RemovedNames, name)
	return nil
}

type mockControlRegistry struct {
	entries map[string]RegistryEntry
}

func (m *mockControlRegistry) List() []RegistryEntry {
	result := make([]RegistryEntry, 0, len(m.entries))
	for _, e := range m.entries {
		result = append(result, e)
	}
	return result
}
func (m *mockControlRegistry) Get(name string) (RegistryEntry, bool) {
	e, ok := m.entries[name]
	return e, ok
}
func (m *mockControlRegistry) Deregister(name string) {}

func TestControlSocket(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	// agent.disable/enable need agent dirs; agent.remove needs zlaw.toml.
	t.Setenv("ZLAW_HOME", dir)

	for _, name := range []string{"alice", "bob", "carol"} {
		agentDir := filepath.Join(dir, "agents", name)
		if err := os.MkdirAll(agentDir, 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte(`[agent]`), 0o600); err != nil {
			t.Fatalf("write agent.toml: %v", err)
		}
	}

	// Create zlaw.toml for agent.remove, agent.disable, and agent.enable.
	zlawTOML := filepath.Join(dir, "zlaw.toml")
	if err := os.WriteFile(zlawTOML, []byte(`[hub]
name = "test"
[[agents]]
name = "alice"
[[agents]]
name = "bob"
`), 0o600); err != nil {
		t.Fatalf("write zlaw.toml: %v", err)
	}

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
		entries: map[string]RegistryEntry{
			"alice": {Name: "alice", Status: AgentConnected, LastHeartbeat: time.Now()},
			"bob":   {Name: "bob", Status: AgentDisconnected, LastHeartbeat: time.Now().Add(-time.Minute)},
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

	t.Run("agent.disable", func(t *testing.T) {
		req := map[string]any{"method": "agent.disable", "params": map[string]any{"name": "alice"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
		if len(sup.StoppedNames) != 1 || sup.StoppedNames[0] != "alice" {
			t.Errorf("StoppedNames = %v, want [alice]", sup.StoppedNames)
		}
	})

	t.Run("agent.enable", func(t *testing.T) {
		// agent.enable clears the disabled flag in zlaw.toml for the named agent.
		// "bob" is registered in the test zlaw.toml, so this should succeed.
		req := map[string]any{"method": "agent.enable", "params": map[string]any{"name": "bob"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
	})

	t.Run("agent.stop", func(t *testing.T) {
		sup.StoppedNames = nil // reset from agent.disable
		req := map[string]any{"method": "agent.stop", "params": map[string]any{"name": "carol"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
		if len(sup.StoppedNames) != 1 || sup.StoppedNames[0] != "carol" {
			t.Errorf("StoppedNames = %v, want [carol]", sup.StoppedNames)
		}
	})

	t.Run("agent.restart", func(t *testing.T) {
		sup.RestartedNames = nil // reset from previous subtests
		req := map[string]any{"method": "agent.restart", "params": map[string]any{"name": "alice"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
		if len(sup.RestartedNames) != 1 || sup.RestartedNames[0] != "alice" {
			t.Errorf("RestartedNames = %v, want [alice]", sup.RestartedNames)
		}
	})

	t.Run("agent.remove", func(t *testing.T) {
		req := map[string]any{"method": "agent.remove", "params": map[string]any{"name": "bob"}}
		data, _ := json.Marshal(req)
		conn.SetWriteDeadline(time.Now().Add(time.Second)) //nolint:errcheck
		conn.SetReadDeadline(time.Now().Add(time.Second))  //nolint:errcheck
		_, _ = conn.Write(append(data, '\n'))
		var raw json.RawMessage
		dec := json.NewDecoder(conn)
		if err := dec.Decode(&raw); err != nil {
			t.Fatalf("read response: %v", err)
		}
		var resp struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !resp.OK {
			t.Fatalf("not ok: %s", resp.Error)
		}
		if len(sup.RemovedNames) != 1 || sup.RemovedNames[0] != "bob" {
			t.Errorf("RemovedNames = %v, want [bob]", sup.RemovedNames)
		}
	})
}
