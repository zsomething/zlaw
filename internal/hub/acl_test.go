package hub_test

import (
	"testing"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

func TestBuildHubACL_AllAgentsHaveEqualPermissions(t *testing.T) {
	acl, err := hub.BuildHubACL([]config.AgentEntry{
		{Name: "specialist"},
		{Name: "worker"},
	})
	if err != nil {
		t.Fatalf("BuildHubACL: %v", err)
	}

	// Users[0] = hub, Users[1] = specialist, Users[2] = worker
	for i := 1; i < len(acl.Users); i++ {
		user := acl.Users[i]
		perms := user.Permissions

		// Every agent can publish to any agent's inbox (P2P delegation).
		hasInboxWildcard := false
		for _, s := range perms.Publish.Allow {
			if s == "agent.*.inbox" {
				hasInboxWildcard = true
				break
			}
		}
		if !hasInboxWildcard {
			t.Errorf("agent %q Publish.Allow missing agent.*.inbox; got %v", user.Username, perms.Publish.Allow)
		}

		// Every agent can publish to registry for heartbeats.
		hasRegistry := false
		for _, s := range perms.Publish.Allow {
			if s == "zlaw.registry" {
				hasRegistry = true
				break
			}
		}
		if !hasRegistry {
			t.Errorf("agent %q Publish.Allow missing zlaw.registry; got %v", user.Username, perms.Publish.Allow)
		}
	}
}

func TestBuildHubACL_AgentCannotPublishHubInbox(t *testing.T) {
	// In the P2P model, no agent should be able to publish to zlaw.hub.inbox
	// (hub management API is CLI-only via control socket).
	acl, err := hub.BuildHubACL([]config.AgentEntry{
		{Name: "specialist"},
	})
	if err != nil {
		t.Fatalf("BuildHubACL: %v", err)
	}

	agent := acl.Users[1]
	for _, s := range agent.Permissions.Publish.Allow {
		if s == "zlaw.hub.inbox" {
			t.Errorf("agent %q should not be able to publish to zlaw.hub.inbox", agent.Username)
		}
	}
}
