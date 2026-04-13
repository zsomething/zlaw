package main

import (
	"bufio"
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
	case "add":
		return runAuthAdd(args[1:])
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
	fmt.Fprintln(os.Stderr, "usage: zlaw-hub auth <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  add      add or update a credential profile")
	fmt.Fprintln(os.Stderr, "  list     list stored credential profiles")
	fmt.Fprintln(os.Stderr, "  remove   remove a credential profile")
}

// runAuthAdd handles: zlaw-hub auth add --provider <name> [--key <key>]
func runAuthAdd(args []string) error {
	fs := flag.NewFlagSet("auth add", flag.ContinueOnError)
	provider := fs.String("provider", "", "provider / profile name (required), e.g. anthropic, openrouter")
	key := fs.String("key", "", "API key (prompted if omitted)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub auth add --provider <name> [--key <key>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *provider == "" {
		return fmt.Errorf("--provider is required (e.g. anthropic, openrouter)")
	}

	apiKey := *key
	if apiKey == "" {
		var err error
		apiKey, err = prompt(fmt.Sprintf("API key for %q: ", *provider), true)
		if err != nil {
			return fmt.Errorf("read api key: %w", err)
		}
	}
	if apiKey == "" {
		return fmt.Errorf("API key must not be empty")
	}

	credPath := auth.DefaultCredentialsPath()
	p := auth.CredentialProfile{
		Type: auth.ProfileTypeAPIKey,
		Key:  apiKey,
	}
	if err := auth.UpsertProfile(credPath, *provider, p); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	fmt.Printf("Saved apikey profile %q to %s\n", *provider, credPath)
	return nil
}

// runAuthList handles: zlaw-hub auth list
func runAuthList(args []string) error {
	fs := flag.NewFlagSet("auth list", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub auth list")
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
		fmt.Printf("Run 'zlaw-hub auth add --provider <name>' to add one. (file: %s)\n", credPath)
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

// runAuthRemove handles: zlaw-hub auth remove --provider <name>
func runAuthRemove(args []string) error {
	fs := flag.NewFlagSet("auth remove", flag.ContinueOnError)
	provider := fs.String("provider", "", "provider / profile name (required)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub auth remove --provider <name>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *provider == "" {
		return fmt.Errorf("--provider is required")
	}

	credPath := auth.DefaultCredentialsPath()
	store, err := auth.LoadStore(credPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	if _, ok := store.Profiles[*provider]; !ok {
		return fmt.Errorf("profile %q not found", *provider)
	}
	delete(store.Profiles, *provider)
	if err := auth.SaveStore(credPath, store); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Printf("Removed profile %q from %s\n", *provider, credPath)
	return nil
}

func profileDetails(p auth.CredentialProfile) string {
	switch p.Type {
	case auth.ProfileTypeAPIKey:
		if len(p.Key) > 8 {
			return "key=" + p.Key[:4] + "..." + p.Key[len(p.Key)-4:]
		}
		return "key=***"
	default:
		return ""
	}
}

// prompt writes label to stdout and reads a line from stdin.
// If sensitive is true, it attempts to disable terminal echo (best-effort).
func prompt(label string, sensitive bool) (string, error) {
	fmt.Print(label)

	if sensitive {
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
		fmt.Println()
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func runStty(arg string) error {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return err
	}
	defer f.Close()

	cmd := exec.Command("stty", arg)
	cmd.Stdin = f
	return cmd.Run()
}
