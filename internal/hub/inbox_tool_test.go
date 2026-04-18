package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

type mockToolSupervisor struct {
	statuses map[string]AgentStatus
}

func (m *mockToolSupervisor) Status(name string) (AgentStatus, error) {
	if s, ok := m.statuses[name]; ok {
		return s, nil
	}
	return AgentStatus{}, nil
}
func (m *mockToolSupervisor) Stop(name string) error    { return nil }
func (m *mockToolSupervisor) Restart(name string) error { return nil }

type mockToolRegistry struct {
	entries map[string]RegistryEntry
}

func (m *mockToolRegistry) List() []RegistryEntry {
	out := make([]RegistryEntry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e)
	}
	return out
}
func (m *mockToolRegistry) Get(name string) (RegistryEntry, bool) {
	e, ok := m.entries[name]
	return e, ok
}

type mockToolHubConfig struct{ name string }

func (m mockToolHubConfig) HubName() string { return m.name }

func TestHubInbox(t *testing.T) {
	sup := &mockToolSupervisor{
		statuses: map[string]AgentStatus{
			"alice": {Name: "alice", Running: true, PID: 1234},
			"bob":   {Name: "bob", Running: false, PID: 0},
		},
	}
	reg := &mockToolRegistry{
		entries: map[string]RegistryEntry{
			"alice": {Name: "alice", Status: AgentConnected},
			"bob":   {Name: "bob", Status: AgentDisconnected},
		},
	}
	cfg := mockToolHubConfig{name: "test-hub"}
	hi := NewHubInbox(sup, reg, cfg, slog.Default())

	ctx := context.Background()

	t.Run("hub_status", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{Tool: "hub_status"})
		if !reply.OK {
			t.Fatalf("hub_status not ok: %s", reply.Error)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if result["name"] != "test-hub" {
			t.Errorf("name = %v, want test-hub", result["name"])
		}
	})

	t.Run("agent_status", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_status",
			Args: map[string]any{"name": "alice"},
		})
		if !reply.OK {
			t.Fatalf("agent_status not ok: %s", reply.Error)
		}
		var result AgentStatus
		if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if result.Name != "alice" {
			t.Errorf("Name = %q, want alice", result.Name)
		}
		if !result.Running {
			t.Error("Running = false, want true")
		}
	})

	t.Run("agent_status missing name", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_status",
			Args: map[string]any{},
		})
		if reply.OK {
			t.Fatal("expected not ok")
		}
	})

	t.Run("agent_stop", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_stop",
			Args: map[string]any{"name": "alice"},
		})
		if !reply.OK {
			t.Fatalf("agent_stop not ok: %s", reply.Error)
		}
	})

	t.Run("agent_restart", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_restart",
			Args: map[string]any{"name": "bob"},
		})
		if !reply.OK {
			t.Fatalf("agent_restart not ok: %s", reply.Error)
		}
	})

	t.Run("agent_list", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{Tool: "agent_list"})
		if !reply.OK {
			t.Fatalf("agent_list not ok: %s", reply.Error)
		}
		var result []RegistryEntry
		if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("Agents len = %d, want 2", len(result))
		}
	})

	t.Run("agent_get", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_get",
			Args: map[string]any{"name": "alice"},
		})
		if !reply.OK {
			t.Fatalf("agent_get not ok: %s", reply.Error)
		}
		var result RegistryEntry
		if err := json.Unmarshal([]byte(reply.Output), &result); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if result.Name != "alice" {
			t.Errorf("Name = %q, want alice", result.Name)
		}
	})

	t.Run("agent_get not found", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "agent_get",
			Args: map[string]any{"name": "nonexistent"},
		})
		if reply.OK {
			t.Fatal("expected not ok for nonexistent agent")
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		reply := hi.HandleToolRequest(ctx, ToolRequest{
			Tool: "nonexistent_tool",
			Args: map[string]any{},
		})
		if reply.OK {
			t.Fatal("expected not ok for unknown tool")
		}
		if reply.Tool != "nonexistent_tool" {
			t.Errorf("Tool = %q, want nonexistent_tool", reply.Tool)
		}
	})
}
