package credentials_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsomething/zlaw/internal/credentials"
)

func TestLoadStore_Missing(t *testing.T) {
	store, err := credentials.LoadStore(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Profiles) != 0 {
		t.Errorf("expected empty profiles, got %d", len(store.Profiles))
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	store := credentials.CredentialStore{
		Profiles: map[string]credentials.CredentialProfile{
			"anthropic": {Name: "anthropic", Data: map[string]string{"api_key": "sk-ant-test"}},
		},
	}
	if err := credentials.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := credentials.LoadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := loaded.Profiles["anthropic"]
	if !ok {
		t.Fatal("profile 'anthropic' not found after reload")
	}
	if p.Data["api_key"] != "sk-ant-test" {
		t.Errorf("api_key = %q, want %q", p.Data["api_key"], "sk-ant-test")
	}
}

func TestUpsertProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")

	if err := credentials.UpsertProfile(path, "anthropic", credentials.CredentialProfile{Name: "anthropic", Data: map[string]string{"api_key": "k1"}}); err != nil {
		t.Fatal(err)
	}
	if err := credentials.UpsertProfile(path, "telegram-bot", credentials.CredentialProfile{Name: "telegram-bot", Data: map[string]string{"telegram_bot_token": "k2"}}); err != nil {
		t.Fatal(err)
	}

	store, _ := credentials.LoadStore(path)
	if len(store.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(store.Profiles))
	}
}

func TestGetProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	store := credentials.CredentialStore{
		Profiles: map[string]credentials.CredentialProfile{
			"test": {Name: "test", Data: map[string]string{"api_key": "secret"}},
		},
	}
	if err := credentials.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	p, err := credentials.GetProfile(path, "test")
	if err != nil {
		t.Fatal(err)
	}
	if p.Data["api_key"] != "secret" {
		t.Errorf("api_key = %q, want %q", p.Data["api_key"], "secret")
	}

	_, err = credentials.GetProfile(path, "missing")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestGetData(t *testing.T) {
	p := credentials.CredentialProfile{
		Name: "test",
		Data: map[string]string{
			"api_key":            "key1",
			"telegram_bot_token": "token2",
		},
	}
	if p.GetData("api_key") != "key1" {
		t.Errorf("GetData(api_key) = %q, want %q", p.GetData("api_key"), "key1")
	}
	if p.GetData("telegram_bot_token") != "token2" {
		t.Errorf("GetData(telegram_bot_token) = %q, want %q", p.GetData("telegram_bot_token"), "token2")
	}
	if p.GetData("missing") != "" {
		t.Errorf("GetData(missing) = %q, want empty", p.GetData("missing"))
	}
}

func TestNewTokenSourceFromStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	store := credentials.CredentialStore{
		Profiles: map[string]credentials.CredentialProfile{
			"anthropic": {Name: "anthropic", Data: map[string]string{"api_key": "sk-ant-test"}},
		},
	}
	if err := credentials.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	src, err := credentials.NewTokenSourceFromStore(path, "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "sk-ant-test" {
		t.Errorf("token = %q, want %q", tok, "sk-ant-test")
	}
}

func TestNewTokenSourceFromStore_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "creds.toml")
	_, err := credentials.NewTokenSourceFromStore(path, "nonexistent")
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestNewTokenSourceFromStore_MissingAPIKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.toml")
	store := credentials.CredentialStore{
		Profiles: map[string]credentials.CredentialProfile{
			"no-key": {Name: "no-key", Data: map[string]string{"other_key": "val"}},
		},
	}
	if err := credentials.SaveStore(path, store); err != nil {
		t.Fatal(err)
	}

	_, err := credentials.NewTokenSourceFromStore(path, "no-key")
	if err == nil {
		t.Error("expected error for profile missing api_key")
	}
}

func TestDefaultCredentialsPath_EnvOverride(t *testing.T) {
	want := "/tmp/test-creds.toml"
	t.Setenv("ZLAW_CREDENTIALS_FILE", want)
	got := credentials.DefaultCredentialsPath()
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
}
