package hub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/nats-io/nats-server/v2/server"

	"github.com/zsomething/zlaw/internal/config"
)

const (
	// hubUsername is the NATS username used by the hub's own internal connection.
	hubUsername = "_hub"
)

// AgentTokens maps agent name → NATS password token.
type AgentTokens map[string]string

// HubACL holds the generated NATS users and the token map for the hub and all agents.
type HubACL struct {
	// Users is the list of server.User entries to pass to server.Options.Users.
	Users []*server.User
	// AgentTokens maps agent name to its NATS token (password).
	AgentTokens AgentTokens
	// HubToken is the NATS token for the hub's own internal connection.
	HubToken string
}

// BuildHubACL generates a per-agent NATS token and permission set for each
// AgentEntry, plus a privileged token for the hub's own internal connection.
//
// Manager agent (entry.Manager == true):
//   - publish:   agent.*.inbox, zlaw.hub.inbox
//   - subscribe: agent.<name>.inbox, zlaw.registry
//
// Specialist agent:
//   - publish:   agent.manager.inbox
//   - subscribe: agent.<name>.inbox
//
// Hub internal (_hub): no permission restrictions (full access).
func BuildHubACL(agents []config.AgentEntry) (*HubACL, error) {
	hubToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("generate hub token: %w", err)
	}

	acl := &HubACL{
		AgentTokens: make(AgentTokens, len(agents)),
		HubToken:    hubToken,
	}

	// Hub internal user — no permission restrictions.
	acl.Users = append(acl.Users, &server.User{
		Username: hubUsername,
		Password: hubToken,
	})

	for _, entry := range agents {
		token, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("generate token for agent %q: %w", entry.Name, err)
		}
		acl.AgentTokens[entry.Name] = token
		acl.Users = append(acl.Users, &server.User{
			Username:    entry.Name,
			Password:    token,
			Permissions: agentPermissions(entry.Name, entry.Manager),
		})
	}

	return acl, nil
}

// agentPermissions returns the NATS subject permissions for the given agent.
//
// All agents publish heartbeats to zlaw.registry, so both manager and
// specialist agents are granted publish access to it.
func agentPermissions(name string, isManager bool) *server.Permissions {
	inboxSubject := "agent." + name + ".inbox"

	if isManager {
		return &server.Permissions{
			Publish: &server.SubjectPermission{
				Allow: []string{
					"agent.*.inbox",
					"zlaw.hub.inbox",
					"zlaw.registry",
					"$JS.API.>", // JetStream management API
					"_INBOX.>",  // NATS inbox for request/reply
				},
			},
			Subscribe: &server.SubjectPermission{
				Allow: []string{
					inboxSubject,
					"zlaw.registry",
					"$JS.API.>", // JetStream API responses
					"_INBOX.>",  // NATS inbox responses
				},
			},
		}
	}

	return &server.Permissions{
		Publish: &server.SubjectPermission{
			Allow: []string{"agent.manager.inbox", "zlaw.registry"},
		},
		Subscribe: &server.SubjectPermission{
			Allow: []string{inboxSubject},
		},
	}
}

// generateToken produces a 32-byte cryptographically random hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
