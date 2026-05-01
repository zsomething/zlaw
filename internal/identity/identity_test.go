package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	seed, pubKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if len(seed) != 32 {
		t.Errorf("seed length = %d, want 32", len(seed))
	}
	if pubKey == "" {
		t.Error("public key is empty")
	}

	// Verify we can generate different keys each time.
	seed3, _, _ := GenerateKeyPair()
	if string(seed) == string(seed3) {
		t.Error("generated same seed twice")
	}
	_ = seed3
}

func TestSignAndVerify(t *testing.T) {
	seed, pubKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	data := []byte("hello world")
	signature, err := Sign(seed, data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if signature == "" {
		t.Error("signature is empty")
	}

	// Verify valid signature.
	if !Verify(pubKey, data, signature) {
		t.Error("Verify returned false for valid signature")
	}

	// Verify invalid signature.
	if Verify(pubKey, data, "invalid-signature") {
		t.Error("Verify returned true for invalid signature")
	}

	// Verify wrong data.
	if Verify(pubKey, []byte("wrong data"), signature) {
		t.Error("Verify returned true for wrong data")
	}
}

func TestResolvePublicKey(t *testing.T) {
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "identity.key")

	// First call should generate a new keypair.
	pubKey1, err := ResolvePublicKey(seedPath)
	if err != nil {
		t.Fatalf("ResolvePublicKey failed: %v", err)
	}
	if pubKey1 == "" {
		t.Error("public key is empty")
	}

	// Verify the file was created.
	if _, err := os.ReadFile(seedPath); err != nil {
		t.Errorf("seed file not created: %v", err)
	}

	// Second call should return the same key.
	pubKey2, err := ResolvePublicKey(seedPath)
	if err != nil {
		t.Fatalf("ResolvePublicKey failed: %v", err)
	}
	if pubKey1 != pubKey2 {
		t.Errorf("public keys don't match: %s != %s", pubKey1, pubKey2)
	}
}

func TestSaveAndLoadSeed(t *testing.T) {
	tmpDir := t.TempDir()
	seedPath := filepath.Join(tmpDir, "test.key")

	// Generate and save.
	seed, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if err := SaveSeed(seedPath, seed); err != nil {
		t.Fatalf("SaveSeed failed: %v", err)
	}

	// Load and verify.
	loaded, err := LoadSeed(seedPath)
	if err != nil {
		t.Fatalf("LoadSeed failed: %v", err)
	}
	if string(loaded) != string(seed) {
		t.Error("loaded seed doesn't match original")
	}
}

func TestVerifyWithInvalidInputs(t *testing.T) {
	// Test empty public key.
	if Verify("", []byte("data"), "signature") {
		t.Error("Verify returned true for empty public key")
	}

	// Test invalid base64 in public key.
	if Verify("not-valid-base64!!!", []byte("data"), "signature") {
		t.Error("Verify returned true for invalid base64 in public key")
	}

	// Test invalid base64 in signature.
	if Verify("dmFsaWQ=", []byte("data"), "not-valid-base64!!!") {
		t.Error("Verify returned true for invalid base64 in signature")
	}
}
