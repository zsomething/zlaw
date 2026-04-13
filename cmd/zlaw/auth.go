package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/chickenzord/zlaw/internal/llm/auth"
)

type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"add or update a credential profile"`
	List   AuthListCmd   `cmd:"" help:"list stored credential profiles"`
	Remove AuthRemoveCmd `cmd:"" help:"remove a credential profile"`
}

// ── auth login ────────────────────────────────────────────────────────────────

type AuthLoginCmd struct {
	Profile      string `required:"" help:"credential profile name"`
	Type         string `required:"" help:"auth type: apikey or oauth2"`
	Key          string `help:"API key (apikey type; prompted if omitted)"`
	TokenURL     string `name:"token-url"     help:"OAuth2 token endpoint URL"`
	ClientID     string `name:"client-id"     help:"OAuth2 client ID"`
	ClientSecret string `name:"client-secret" help:"OAuth2 client secret"`
	Scope        string `help:"OAuth2 scope (optional)"`
}

func (c *AuthLoginCmd) Run(ctx context.Context) error {
	credPath := auth.DefaultCredentialsPath()
	switch auth.ProfileType(c.Type) {
	case auth.ProfileTypeAPIKey:
		return loginAPIKey(credPath, c.Profile, c.Key)
	case auth.ProfileTypeOAuth2:
		return loginOAuth2(ctx, credPath, c.Profile, c.TokenURL, c.ClientID, c.ClientSecret, c.Scope)
	default:
		return fmt.Errorf("unknown auth type %q (supported: apikey, oauth2)", c.Type)
	}
}

func loginAPIKey(credPath, profileName, key string) error {
	if key == "" {
		var err error
		key, err = prompt("API key: ", true)
		if err != nil {
			return fmt.Errorf("read api key: %w", err)
		}
	}
	if key == "" {
		return fmt.Errorf("API key must not be empty")
	}

	p := auth.CredentialProfile{
		Type: auth.ProfileTypeAPIKey,
		Key:  key,
	}
	if err := auth.UpsertProfile(credPath, profileName, p); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	fmt.Printf("Saved apikey profile %q to %s\n", profileName, credPath)
	return nil
}

func loginOAuth2(ctx context.Context, credPath, profileName, tokenURL, clientID, clientSecret, scope string) error {
	var err error
	if tokenURL == "" {
		tokenURL, err = prompt("Token URL: ", false)
		if err != nil {
			return fmt.Errorf("read token_url: %w", err)
		}
	}
	if clientID == "" {
		clientID, err = prompt("Client ID: ", false)
		if err != nil {
			return fmt.Errorf("read client_id: %w", err)
		}
	}
	if clientSecret == "" {
		clientSecret, err = prompt("Client secret: ", true)
		if err != nil {
			return fmt.Errorf("read client_secret: %w", err)
		}
	}

	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return fmt.Errorf("token_url, client_id, and client_secret are required")
	}

	p := auth.CredentialProfile{
		Type:         auth.ProfileTypeOAuth2,
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scope:        scope,
	}

	fmt.Print("Validating credentials... ")
	src, err := auth.NewTokenSource(p)
	if err != nil {
		return fmt.Errorf("build token source: %w", err)
	}
	if _, err := src.Token(ctx); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("validate oauth2 credentials: %w", err)
	}
	fmt.Println("ok")

	if err := auth.UpsertProfile(credPath, profileName, p); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	fmt.Printf("Saved oauth2 profile %q to %s\n", profileName, credPath)
	return nil
}

// ── auth list ─────────────────────────────────────────────────────────────────

type AuthListCmd struct{}

func (c *AuthListCmd) Run() error {
	credPath := auth.DefaultCredentialsPath()
	store, err := auth.LoadStore(credPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if len(store.Profiles) == 0 {
		fmt.Println("No credential profiles found.")
		fmt.Printf("Run 'zlaw auth login --profile <name> --type apikey' to add one. (file: %s)\n", credPath)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROFILE\tTYPE\tDETAILS")
	for name, p := range store.Profiles {
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, p.Type, profileDetails(p))
	}
	w.Flush()
	return nil
}

// ── auth remove ───────────────────────────────────────────────────────────────

type AuthRemoveCmd struct {
	Profile string `required:"" help:"profile name to remove"`
}

func (c *AuthRemoveCmd) Run() error {
	credPath := auth.DefaultCredentialsPath()
	store, err := auth.LoadStore(credPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if _, ok := store.Profiles[c.Profile]; !ok {
		return fmt.Errorf("profile %q not found", c.Profile)
	}
	delete(store.Profiles, c.Profile)
	if err := auth.SaveStore(credPath, store); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Printf("Removed profile %q from %s\n", c.Profile, credPath)
	return nil
}
