package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zsomething/zlaw/internal/hub"
)

// mockAgentRegistry is a test double for AgentRegistry.
type mockAgentRegistry struct {
	entries []hub.RegistryEntry
	err     error
}

func (m *mockAgentRegistry) ListAgents(ctx context.Context) ([]hub.RegistryEntry, error) {
	return m.entries, m.err
}

func (m *mockAgentRegistry) GetAgent(ctx context.Context, name string) (*hub.RegistryEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	for i := range m.entries {
		if m.entries[i].Name == name {
			return &m.entries[i], nil
		}
	}
	return nil, nil
}

func TestAgentList_Execute(t *testing.T) {
	mock := &mockAgentRegistry{
		entries: []hub.RegistryEntry{
			{Name: "coding", Version: "1.0", Capabilities: []string{"bash"}, Roles: []string{"coding"}},
			{Name: "assistant", Version: "2.0", Roles: []string{"general"}},
		},
	}
	tool := &AgentList{Registry: mock}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["count"].(float64) != 2 {
		t.Errorf("count = %v, want 2", out["count"])
	}
}

func TestAgentList_ExecuteNilRegistry(t *testing.T) {
	tool := &AgentList{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestAgentList_ExecuteError(t *testing.T) {
	mock := &mockAgentRegistry{err: errors.New("network error")}
	tool := &AgentList{Registry: mock}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAgentGet_Execute(t *testing.T) {
	mock := &mockAgentRegistry{
		entries: []hub.RegistryEntry{
			{Name: "coding", Version: "1.0", Capabilities: []string{"bash"}, Roles: []string{"coding"}},
		},
	}
	tool := &AgentGet{Registry: mock}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"id":"coding"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var entry hub.RegistryEntry
	if err := json.Unmarshal([]byte(result), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Name != "coding" {
		t.Errorf("name = %q, want coding", entry.Name)
	}
	if entry.Version != "1.0" {
		t.Errorf("version = %q, want 1.0", entry.Version)
	}
}

func TestAgentGet_ExecuteMissingID(t *testing.T) {
	tool := &AgentGet{Registry: &mockAgentRegistry{}}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestAgentGet_ExecuteNotFound(t *testing.T) {
	tool := &AgentGet{Registry: &mockAgentRegistry{entries: []hub.RegistryEntry{}}}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"unknown"}`))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestAgentGet_ExecuteNilRegistry(t *testing.T) {
	tool := &AgentGet{}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"x"}`))
	if err == nil {
		t.Fatal("expected error for nil registry")
	}
}

func TestNATSAgentRegistry_ListAgents_NilMessenger(t *testing.T) {
	reg := NewAgentRegistry(nil)
	_, err := reg.ListAgents(context.Background())
	if err == nil {
		t.Fatal("expected error for nil messenger")
	}
}
