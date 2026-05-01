// Package identity provides agent keypair generation, signing, and verification
// for secure inter-agent communication over NATS.
package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

// GenerateKeyPair creates a new Ed25519 keypair for an agent.
// Returns the raw seed (for storage) and the public key (for identification).
func GenerateKeyPair() (seed []byte, publicKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, "", fmt.Errorf("generate keypair: %w", err)
	}
	seed = priv.Seed()
	publicKey = base64.StdEncoding.EncodeToString(pub)
	return seed, publicKey, nil
}

// SaveSeed writes the keypair seed to a file with restricted permissions (0600).
func SaveSeed(path string, seed []byte) error {
	// Ensure parent directory exists.
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create identity dir: %w", err)
	}
	return os.WriteFile(path, seed, 0o600)
}

// LoadSeed reads a keypair seed from file.
func LoadSeed(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ResolvePublicKey returns the public key for a seed at path.
// If the seed file doesn't exist, generates a new keypair and saves it.
func ResolvePublicKey(seedPath string) (string, error) {
	seed, err := LoadSeed(seedPath)
	if os.IsNotExist(err) {
		// Generate new keypair and save.
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
		return "", fmt.Errorf("load seed: %w", err)
	}

	// Derive public key from seed.
	if len(seed) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid seed size: expected %d, got %d", ed25519.SeedSize, len(seed))
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))
	return publicKey, nil
}

// Sign signs data with the given seed and returns a base64-encoded signature.
func Sign(seed []byte, data []byte) (string, error) {
	if len(seed) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid seed size: expected %d, got %d", ed25519.SeedSize, len(seed))
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	signature := ed25519.Sign(privateKey, data)
	return base64.StdEncoding.EncodeToString(signature), nil
}

// Verify verifies a base64-encoded signature against data using the public key.
// Returns true if the signature is valid, false otherwise.
func Verify(publicKeyBase64 string, data []byte, signatureBase64 string) bool {
	pubBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return false
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	publicKey := ed25519.PublicKey(pubBytes)
	return ed25519.Verify(publicKey, data, sigBytes)
}

// DefaultSeedPath returns the default path for an agent's identity seed.
func DefaultSeedPath() string {
	return filepath.Join(config.AgentHome(), "identity.key")
}
