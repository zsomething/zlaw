package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/chickenzord/zlaw/internal/llm/auth"
)

func runAuth(args []string) error {
	if len(args) == 0 {
		printAuthUsage()
		return nil
	}

	switch args[0] {
	case "login":
		return runAuthLogin(args[1:])
	case "list":
		return runAuthList(args[1:])
	case "remove":
		return runAuthRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown auth command %q\n", args[0])
		printAuthUsage()
		os.Exit(1)
		return nil
	}
}

func printAuthUsage() {
	fmt.Fprintln(os.Stderr, "usage: zlaw-agent auth <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  login    add or update a credential profile")
	fmt.Fprintln(os.Stderr, "  list     list stored credential profiles")
	fmt.Fprintln(os.Stderr, "  remove   remove a credential profile")
}

// runAuthLogin handles: zlaw-agent auth login --profile <name> --type <apikey|oauth2> [flags]
func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	profile := fs.String("profile", "", "credential profile name (required)")
	authType := fs.String("type", "", "auth type: apikey or oauth2 (required)")
	// apikey flags
	apiKey := fs.String("key", "", "API key (apikey type; prompted if omitted)")
	// oauth2 flags
	tokenURL := fs.String("token-url", "", "OAuth2 token endpoint URL")
	clientID := fs.String("client-id", "", "OAuth2 client ID")
	clientSecret := fs.String("client-secret", "", "OAuth2 client secret")
	scope := fs.String("scope", "", "OAuth2 scope (optional)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-agent auth login --profile <name> --type <apikey|oauth2> [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *profile == "" {
		return fmt.Errorf("--profile is required")
	}
	if *authType == "" {
		return fmt.Errorf("--type is required (apikey or oauth2)")
	}

	credPath := auth.DefaultCredentialsPath()

	switch auth.ProfileType(*authType) {
	case auth.ProfileTypeAPIKey:
		return loginAPIKey(credPath, *profile, *apiKey)
	case auth.ProfileTypeOAuth2:
		return loginOAuth2(credPath, *profile, *tokenURL, *clientID, *clientSecret, *scope)
	default:
		return fmt.Errorf("unknown auth type %q (supported: apikey, oauth2)", *authType)
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

func loginOAuth2(credPath, profileName, tokenURL, clientID, clientSecret, scope string) error {
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

	// Validate by fetching a token before saving.
	fmt.Print("Validating credentials... ")
	src, err := auth.NewTokenSource(p)
	if err != nil {
		return fmt.Errorf("build token source: %w", err)
	}
	if _, err := src.Token(context.Background()); err != nil {
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

// runAuthList handles: zlaw-agent auth list
func runAuthList(args []string) error {
	fs := flag.NewFlagSet("auth list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-agent auth list")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	credPath := auth.DefaultCredentialsPath()
	store, err := auth.LoadStore(credPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if len(store.Profiles) == 0 {
		fmt.Println("No credential profiles found.")
		fmt.Printf("Run 'zlaw-agent auth login' to add one. (file: %s)\n", credPath)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROFILE\tTYPE\tDETAILS")
	for name, p := range store.Profiles {
		details := profileDetails(p)
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, p.Type, details)
	}
	w.Flush()
	return nil
}

func profileDetails(p auth.CredentialProfile) string {
	switch p.Type {
	case auth.ProfileTypeAPIKey:
		if len(p.Key) > 8 {
			return "key=" + p.Key[:4] + "..." + p.Key[len(p.Key)-4:]
		}
		return "key=***"
	case auth.ProfileTypeOAuth2:
		parts := []string{"token_url=" + p.TokenURL, "client_id=" + p.ClientID}
		if p.Scope != "" {
			parts = append(parts, "scope="+p.Scope)
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

// runAuthRemove handles: zlaw-agent auth remove --profile <name>
func runAuthRemove(args []string) error {
	fs := flag.NewFlagSet("auth remove", flag.ContinueOnError)
	profile := fs.String("profile", "", "credential profile name (required)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-agent auth remove --profile <name>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profile == "" {
		return fmt.Errorf("--profile is required")
	}

	credPath := auth.DefaultCredentialsPath()
	store, err := auth.LoadStore(credPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if _, ok := store.Profiles[*profile]; !ok {
		return fmt.Errorf("profile %q not found", *profile)
	}
	delete(store.Profiles, *profile)
	if err := auth.SaveStore(credPath, store); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Printf("Removed profile %q from %s\n", *profile, credPath)
	return nil
}

// prompt writes label to stdout and reads a line from stdin.
// If sensitive is true, it attempts to disable terminal echo (best-effort).
func prompt(label string, sensitive bool) (string, error) {
	fmt.Print(label)

	if sensitive {
		// Best-effort: suppress echo on Unix by calling stty directly.
		// We don't add a dependency on golang.org/x/term here.
		_ = runStty("-echo")
		defer func() { _ = runStty("echo") }()
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("unexpected EOF")
	}

	if sensitive {
		fmt.Println() // newline after hidden input
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func runStty(arg string) error {
	// Use /dev/tty so it works even when stdin is redirected.
	f, err := os.Open("/dev/tty")
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("stty", arg)
	cmd.Stdin = f
	return cmd.Run()
}
