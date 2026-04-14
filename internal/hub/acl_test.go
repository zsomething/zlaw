package hub_test

import (
	"testing"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/hub"
)

func TestBuildHubACL_SpecialistCanPublishRegistry(t *testing.T) {
	acl, err := hub.BuildHubACL([]config.AgentEntry{
		{Name: "specialist"},
	})
	if err != nil {
		t.Fatalf("BuildHubACL: %v", err)
	}

	spec := acl.Users[1] // [0] = hub, [1] = specialist
	if spec.Username != "specialist" {
		t.Fatalf("expected user[1] to be specialist, got %q", spec.Username)
	}

	// Specialist must be able to publish to zlaw.registry for heartbeats.
	for _, subject := range spec.Permissions.Publish.Allow {
		if subject == "zlaw.registry" {
			return
		}
	}
	t.Errorf("specialist Publish.Allow = %v; want zlaw.registry included",
		spec.Permissions.Publish.Allow)
}

func TestBuildHubACL_ManagerPermissions(t *testing.T) {
	acl, err := hub.BuildHubACL([]config.AgentEntry{
		{Name: "manager", Manager: true},
	})
	if err != nil {
		t.Fatalf("BuildHubACL: %v", err)
	}

	mgr := acl.Users[1]
	if mgr.Permissions.Publish == nil {
		t.Fatal("manager has no Publish permissions")
	}

	publishAllow := mgr.Permissions.Publish.Allow
	wantPublish := []string{"agent.*.inbox", "zlaw.hub.inbox", "zlaw.registry"}
	for _, s := range wantPublish {
		found := false
		for _, a := range publishAllow {
			if a == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manager Publish.Allow missing %q; got %v", s, publishAllow)
		}
	}
}
