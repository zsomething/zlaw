package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/llm/auth"
)

// ---- credential store tests ----

func TestLoadStore_Missing(t *testing.T) {
	store, err := auth.LoadStore(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Profiles) != 0 {
		t.Errorf("expected empty profiles, got %d", len(store.Profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	store := auth.CredentialStore{
		Profiles: map[string]auth.CredentialProfile{
			"my-key": {Type: auth.ProfileTypeAPIKey, Key: "sk-test"},
		},
	}
	if err := auth.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := auth.LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := loaded.Profiles["my-key"]
	if !ok {
		t.Fatal("profile 'my-key' not found after reload")
	}
	if p.Key != "sk-test" {
		t.Errorf("key = %q, want %q", p.Key, "sk-test")
	}
}

func TestUpsertProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")

	if err := auth.UpsertProfile(path, "p1", auth.CredentialProfile{Type: auth.ProfileTypeAPIKey, Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := auth.UpsertProfile(path, "p2", auth.CredentialProfile{Type: auth.ProfileTypeAPIKey, Key: "k2"}); err != nil {
		t.Fatal(err)
	}

	store, _ := auth.LoadStore(path)
	if len(store.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(store.Profiles))
	}
}

// ---- TokenSource tests ----

func TestStaticKeySource(t *testing.T) {
	src, err := auth.NewTokenSource(auth.CredentialProfile{Type: auth.ProfileTypeAPIKey, Key: "sk-hello"})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "sk-hello" {
		t.Errorf("token = %q, want %q", tok, "sk-hello")
	}
}

func TestStaticKeySource_EmptyKey(t *testing.T) {
	_, err := auth.NewTokenSource(auth.CredentialProfile{Type: auth.ProfileTypeAPIKey})
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestOAuth2Source_UsesStoredToken(t *testing.T) {
	expiry := time.Now().Add(10 * time.Minute)
	src, err := auth.NewTokenSource(auth.CredentialProfile{
		Type:        auth.ProfileTypeOAuth2,
		AccessToken: "stored-token",
		Expiry:      expiry,
	})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "stored-token" {
		t.Errorf("token = %q, want %q", tok, "stored-token")
	}
}

func TestOAuth2Source_FetchesWhenExpired(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fresh-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	src, err := auth.NewTokenSource(auth.CredentialProfile{
		Type:         auth.ProfileTypeOAuth2,
		AccessToken:  "expired-token",
		Expiry:       time.Now().Add(-1 * time.Minute), // already expired
		TokenURL:     srv.URL,
		ClientID:     "client",
		ClientSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "fresh-token" {
		t.Errorf("token = %q, want %q", tok, "fresh-token")
	}
	if callCount != 1 {
		t.Errorf("token endpoint called %d times, want 1", callCount)
	}
}

func TestOAuth2Source_CachesToken(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	src, err := auth.NewTokenSource(auth.CredentialProfile{
		Type:         auth.ProfileTypeOAuth2,
		TokenURL:     srv.URL,
		ClientID:     "c",
		ClientSecret: "s",
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			src.Token(context.Background()) //nolint:errcheck
		}()
	}
	wg.Wait()

	// Allow 1 fetch (first call); all subsequent should hit cache.
	if callCount > 1 {
		t.Errorf("token endpoint called %d times (concurrent), want 1", callCount)
	}
}

func TestNewTokenSourceFromStore_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.toml")
	_, err := auth.NewTokenSourceFromStore(path, "nonexistent")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestDefaultCredentialsPath_EnvOverride(t *testing.T) {
	want := "/tmp/test-creds.toml"
	t.Setenv("ZLAW_CREDENTIALS_FILE", want)
	got := auth.DefaultCredentialsPath()
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
