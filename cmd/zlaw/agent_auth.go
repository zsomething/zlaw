package main

import (
	"bufio"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/zsomething/zlaw/internal/secrets"
)

type AgentAuthCmd struct {
	Add    AgentAuthAddCmd    `cmd:"" help:"add a secret"`
	List   AgentAuthListCmd   `cmd:"" help:"list secret names"`
	Remove AgentAuthRemoveCmd `cmd:"" help:"remove a secret"`
}

// ── zlaw auth add ──────────────────────────────────────────────────────────────

type AgentAuthAddCmd struct {
	Name  string `arg:"true" help:"secret name (e.g., MINIMAX_API_KEY)"`
	Value string `help:"secret value (will prompt if not provided)"`
}

func (c *AgentAuthAddCmd) Run() error {
	path := secrets.DefaultSecretsPath()

	// Prompt for value if not provided
	value := c.Value
	if value == "" {
		fmt.Printf("Enter secret value for %s: ", c.Name)
		reader := bufio.NewReader(os.Stdin)
		val, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read secret: %w", err)
		}
		value = val[:len(val)-1] // trim newline
	}

	// Load existing or create new store
	store, err := secrets.Load(path)
	if err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}

	// Set the secret
	store.Set(c.Name, value)

	if err := secrets.Save(path, store); err != nil {
		return fmt.Errorf("save secrets: %w", err)
	}

	fmt.Printf("Added secret %q to %s\n", c.Name, path)
	return nil
}

// ── zlaw auth list ─────────────────────────────────────────────────────────

type AgentAuthListCmd struct{}

func (c *AgentAuthListCmd) Run() error {
	path := secrets.DefaultSecretsPath()

	store, err := secrets.Load(path)
	if err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}

	if len(store) == 0 {
		fmt.Println("No secrets found.")
		fmt.Printf("Secrets file: %s\n", path)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME")
	for _, name := range store.List() {
		fmt.Fprintf(w, "%s\n", name)
	}
	return w.Flush()
}

// ── zlaw auth remove ─────────────────────────────────────────────────────────

type AgentAuthRemoveCmd struct {
	Name string `arg:"true" help:"secret name to remove"`
}

func (c *AgentAuthRemoveCmd) Run() error {
	path := secrets.DefaultSecretsPath()

	store, err := secrets.Load(path)
	if err != nil {
		return fmt.Errorf("load secrets: %w", err)
	}

	if _, ok := store[c.Name]; !ok {
		return fmt.Errorf("secret %q not found", c.Name)
	}

	delete(store, c.Name)

	if err := secrets.Save(path, store); err != nil {
		return fmt.Errorf("save secrets: %w", err)
	}

	fmt.Printf("Removed secret %q from %s\n", c.Name, path)
	return nil
}
