# Phase 3: Identity System

## Goal

Complete the stub `internal/identity` package to provide agent keypair generation, NATS authentication, and message signing/verification.

## Design

From `docs/design/security.md`:
- Each agent has a keypair (NKeys)
- Hub verifies identity on connect
- Messages are signed

## Changes

### 1. Keypair generation at agent creation time

In `internal/identity/identity.go`:

```go
package identity

import (
    "github.com/nats-io/nkeys"
)

// GenerateKeyPair creates a new NKey keypair for an agent.
// Returns the raw seed (for storage) and the public key (for NATS auth).
func GenerateKeyPair() (seed []byte, publicKey string, err error) {
    kp, err := nkeys.CreatePair(nkeys.PrefixByteOperator) // operator key for signing
    if err != nil {
        return nil, "", err
    }
    return kp.Seed(), kp.Public(), nil
}

// SaveSeed writes the keypair seed to a file with restricted permissions.
func SaveSeed(path string, seed []byte) error {
    return os.WriteFile(path, seed, 0600)
}

// LoadSeed reads a keypair seed from file.
func LoadSeed(path string) ([]byte, error) {
    return os.ReadFile(path)
}

// ResolvePublicKey returns the public key for a seed at path.
// If seed doesn't exist, generates a new keypair.
func ResolvePublicKey(seedPath string) (string, error) {
    seed, err := LoadSeed(seedPath)
    if os.IsNotExist(err) {
        seed, pub, err := GenerateKeyPair()
        if err != nil {
            return "", err
        }
        if err := SaveSeed(seedPath, seed); err != nil {
            return "", err
        }
        return pub, nil
    }
    if err != nil {
        return "", err
    }
    kp, err := nkeys.FromSeed(seed)
    if err != nil {
        return "", err
    }
    return kp.Public(), nil
}
```

### 2. Message signing

```go
import "github.com/nats-io/nkeys"

// Sign signs data with the given seed.
func Sign(seed []byte, data []byte) ([]byte, error) {
    kp, err := nkeys.FromSeed(seed)
    if err != nil {
        return nil, err
    }
    sig, err := kp.Sign(data)
    if err != nil {
        return nil, err
    }
    return sig, nil
}

// Verify verifies a signature against data using the public key.
func Verify(publicKey string, data, signature []byte) bool {
    kp, err := nkeys.FromPublic(publicKey)
    if err != nil {
        return false
    }
    return kp.Verify(data, signature)
}
```

### 3. Integration with Hub client connection

In `internal/agent/hubclient.go`:

```go
// After NATS connection, authenticate with credentials:
seedPath := filepath.Join(config.AgentHome(), "identity.key")
seed, err := identity.LoadSeed(seedPath)
if err == nil {
    // Use seed for NATS authentication (nkeys authentication)
    natsOpts = append(natsOpts, nats.NkeyOptionFromSeed(seed))
}
```

### 4. Integration with Hub ACL

In `internal/hub/acl.go`:

```go
// On agent connect, verify the public key matches expected from registry
func VerifyAgentConnection(publicKey string, expected string) bool {
    return publicKey == expected
}
```

### 5. Message envelope signing

In `internal/messaging/envelope.go`:

```go
type TaskEnvelope struct {
    // ... existing fields ...
    // NEW:
    Signature string `json:"signature,omitempty"`
}

// Sign signs the envelope with the agent's private key.
// Creates a deterministic payload from From + To + Task + SessionID for signing.
func (e *TaskEnvelope) Sign(seed []byte) error {
    payload := e.From + e.To + e.Task + e.SessionID
    sig, err := identity.Sign(seed, []byte(payload))
    if err != nil {
        return err
    }
    e.Signature = base64.StdEncoding.EncodeToString(sig)
    return nil
}

// Verify checks the signature against the sender's public key.
func (e *TaskEnvelope) Verify(publicKey string) bool {
    if e.Signature == "" {
        return false // no signature = unverified
    }
    sig, err := base64.StdEncoding.DecodeString(e.Signature)
    if err != nil {
        return false
    }
    payload := e.From + e.To + e.Task + e.SessionID
    return identity.Verify(publicKey, []byte(payload), sig)
}
```

### 6. Hub verification at inbox handling

In `internal/hub/inbox.go` — verify signature before processing:

```go
func (h *HubInbox) processMessage(data []byte) {
    var env TaskEnvelope
    if err := json.Unmarshal(data, &env); err != nil {
        // ...
    }
    
    // Verify signature against sender's public key from registry
    entry, ok := h.registry.Get(env.From)
    if !ok {
        // Unknown sender - reject
        return
    }
    if !env.Verify(entry.PublicKey) {
        // Signature mismatch - log and reject
        h.logger.Warn("signature verification failed", "from", env.From)
        return
    }
    
    // Proceed with processing
}
```

### 7. Storage

Keypairs stored in agent directory:
```
$ZLAW_AGENT_HOME/
├── agent.toml
├── credentials.toml
├── identity.key    # NKey seed, 0600 permissions
└── ...
```

Hub stores public keys in registry:
```go
type RegistryEntry struct {
    Name         string
    PublicKey    string  // NEW
    // ...
}
```

## File Changes

| File | Action |
|------|--------|
| `internal/identity/identity.go` | Implement keypair generation, signing, verification |
| `internal/messaging/envelope.go` | Add signature field and Sign/Verify methods |
| `internal/agent/hubclient.go` | Load seed and use for NATS NKey auth |
| `internal/hub/registry.go` | Add PublicKey field to RegistryEntry |
| `internal/hub/acl.go` | Verify key on connect |
| `internal/hub/inbox.go` | Verify envelope signature before processing |

## Verification

1. Keypair generated on first agent run
2. NATS connection uses NKey authentication
3. Delegation envelopes include signature
4. Hub verifies signature before processing delegation
5. Invalid signatures are rejected
6. `go build ./...` passes
7. Existing tests pass

## Dependencies

- Phase 2: Auth profiles must be working because identity extends the registration flow
- Phase 1: Hub must not read agent filesystem (identity.key is read by agent, not hub)

## Non-Goals

- Revocation / key rotation (future)
- Multiple keys per agent (future)
- Key escrow (future)