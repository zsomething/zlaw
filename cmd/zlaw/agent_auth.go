package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
)

type AgentAuthCmd struct {
	Set    AgentAuthSetCmd    `cmd:"" help:"set a credential value for a profile"`
	List   AgentAuthListCmd   `cmd:"" help:"list credential profiles"`
	Remove AgentAuthRemoveCmd `cmd:"" help:"remove a credential profile"`
}

// ── hub auth set ───────────────────────────────────────────────────────────────

type AgentAuthSetCmd struct {
	Agent   string `required:"" help:"agent id"`
	Profile string `required:"" help:"profile name (e.g., anthropic, telegram, fizzy)"`
	Key     string `required:"" help:"credential value or secret key"`
}

func (c *AgentAuthSetCmd) Run() error {
	agentDir := resolveAgentDir(c.Agent)
	credsPath := agentDir + "/credentials.toml"

	// Load existing or create new store.
	store, err := credentials.LoadStore(credsPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	// Determine the key name based on profile naming convention.
	keyName := "api_key"
	switch c.Profile {
	case "anthropic", "openrouter", "openai":
		keyName = "api_key"
	case "telegram":
		keyName = "telegram_bot_token"
	case "fizzy":
		keyName = "fizzy_api_key"
	}

	// Upsert the profile.
	profile := store.Profiles[c.Profile]
	if profile.Name == "" {
		profile.Name = c.Profile
	}
	if profile.Data == nil {
		profile.Data = make(map[string]string)
	}
	profile.Data[keyName] = c.Key
	store.Profiles[c.Profile] = profile

	if err := credentials.SaveStore(credsPath, store); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("Set %s.%s in %s\n", c.Profile, keyName, credsPath)
	return nil
}

// ── hub auth list ─────────────────────────────────────────────────────────────

type AgentAuthListCmd struct {
	Agent string `required:"" help:"agent id"`
}

func (c *AgentAuthListCmd) Run() error {
	agentDir := resolveAgentDir(c.Agent)
	credsPath := agentDir + "/credentials.toml"

	store, err := credentials.LoadStore(credsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No credentials found for agent %q\n", c.Agent)
			return nil
		}
		return fmt.Errorf("load credentials: %w", err)
	}

	if len(store.Profiles) == 0 {
		fmt.Printf("No credential profiles found for agent %q\n", c.Agent)
		fmt.Printf("Credentials file: %s\n", credsPath)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "PROFILE\tKEYS\n")
	for name, p := range store.Profiles {
		var keys []string
		for k := range p.Data {
			keys = append(keys, k)
		}
		fmt.Fprintf(w, "%s\t%s\n", name, join(keys, ", "))
	}
	w.Flush()
	return nil
}

// ── hub auth remove ───────────────────────────────────────────────────────────

type AgentAuthRemoveCmd struct {
	Agent   string `required:"" help:"agent id"`
	Profile string `required:"" help:"profile name to remove"`
}

func (c *AgentAuthRemoveCmd) Run() error {
	agentDir := resolveAgentDir(c.Agent)
	credsPath := agentDir + "/credentials.toml"

	store, err := credentials.LoadStore(credsPath)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	if _, ok := store.Profiles[c.Profile]; !ok {
		return fmt.Errorf("profile %q not found", c.Profile)
	}

	delete(store.Profiles, c.Profile)

	if err := credentials.SaveStore(credsPath, store); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("Removed profile %q from %s\n", c.Profile, credsPath)
	return nil
}

// resolveAgentDir returns the agent directory for a given agent name.
func resolveAgentDir(agentName string) string {
	home := config.ZlawHome()
	return home + "/agents/" + agentName
}

func join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
