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
// In the P2P delegation model all agents have equal permissions:
//   - publish: agent.*.inbox, agent.manager.inbox, zlaw.registry, zlaw.registry.list, _INBOX.>, $JS.ACK.>, $JS.API.>
//   - subscribe: zlaw.registry, agent.<name>.inbox, _INBOX.>, $JS.API.>
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
			Permissions: agentPermissions(entry.Name),
		})
	}

	return acl, nil
}

// agentPermissions returns the NATS subject permissions for all agents.
// In the P2P delegation model all agents need:
//   - subscribe:  zlaw.registry        (receive heartbeats / registration messages)
//   - publish:    agent.*.inbox        (P2P delegation)
//   - publish:    zlaw.registry        (send heartbeats)
//   - subscribe:  agent.<name>.inbox   (own inbox for JetStream durable consumer)
//   - publish/sub: $JS.API.>          (JetStream: create consumers, fetch, info, etc.)
//   - subscribe:  _INBOX.>             (NATS reply subjects for request/reply)
func agentPermissions(name string) *server.Permissions {
	inboxSubject := "agent." + name + ".inbox"
	return &server.Permissions{
		Publish: &server.SubjectPermission{
			Allow: []string{
				"agent.*.inbox",       // P2P: send delegation to any agent
				"agent.manager.inbox", // reply inbox for delegations
				"zlaw.registry",       // heartbeat / registry
				"zlaw.registry.list",  // registry query
				"_INBOX.>",            // NATS reply subjects (delegate reply, etc.)
				"$JS.API.>",           // JetStream management API
				"$JS.ACK.>",           // JetStream message acks
				"_INBOX.delegate.>",   // delegate reply subjects
			},
		},
		Subscribe: &server.SubjectPermission{
			Allow: []string{
				"zlaw.registry", // heartbeat subscription / registry messages
				inboxSubject,    // own inbox (JetStream durable consumer)
				"$JS.API.>",     // JetStream API responses
				"_INBOX.>",      // NATS reply subjects
			},
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
