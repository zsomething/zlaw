package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm/auth"
)

// ── Templates ────────────────────────────────────────────────────────────────

const zlawTOMLTemplate = `[hub]
name = "main"
description = "zlaw hub"

# Agents supervised by this hub.
# List agent names here; each maps to $ZLAW_HOME/agents/<name>/.
[agents]
names = ["manager"]

[nats]
# Embedded NATS server listen address. Defaults to 127.0.0.1:4222.
# listen = "127.0.0.1:4222"
`

const credentialsTemplate = `# Credential profiles for zlaw and its agents.
# Add profiles with: zlaw auth add --provider <name>

[profiles]
`

// agentTOMLTemplate is used for both the default manager and any named agent.
// %s is substituted with the agent name.
const agentTOMLTemplate = `[agent]
name = %q
description = ""

[llm]
backend = "anthropic"
model = "claude-opus-4-5"
auth_profile = "anthropic"
max_tokens = 8192
timeout_sec = 120

[tools]
# allowed = ["bash", "read_file", ...]  # uncomment to restrict tool access

[adapter]
type = "cli"
`

const soulMDTemplate = `You are a helpful personal assistant.
`

// identityMDTemplate has the agent name substituted for %s.
const identityMDTemplate = `# Identity

Your name is %s.
`

// ── Command ──────────────────────────────────────────────────────────────────

// InitCmd bootstraps $ZLAW_HOME.
//
// Without --agent: full workspace bootstrap (zlaw.toml, credentials.toml,
// agents/manager/).
//
// With --agent <name>: create a named agent directory only (no hub config).
type InitCmd struct {
	Agent string `short:"a" help:"create a named agent instead of bootstrapping the full workspace"`
	Force bool   `help:"overwrite existing files"`
}

func (c *InitCmd) Run() error {
	if c.Agent != "" {
		return c.initAgent(c.Agent)
	}
	return c.initWorkspace()
}

// initWorkspace creates the full $ZLAW_HOME layout: zlaw.toml, credentials,
// and a default manager agent.
func (c *InitCmd) initWorkspace() error {
	home := config.ZlawHome()
	credPath := auth.DefaultCredentialsPath()
	managerDir := filepath.Join(home, "agents", "manager")

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
		{filepath.Join(home, "zlaw.toml"), zlawTOMLTemplate, 0o600},
		{credPath, credentialsTemplate, 0o600},
		{filepath.Join(managerDir, "agent.toml"), fmt.Sprintf(agentTOMLTemplate, "manager"), 0o600},
		{filepath.Join(managerDir, "SOUL.md"), soulMDTemplate, 0o644},
		{filepath.Join(managerDir, "IDENTITY.md"), fmt.Sprintf(identityMDTemplate, "Manager"), 0o644},
	}

	if !c.Force {
		for _, f := range files {
			if _, err := os.Stat(f.path); err == nil {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", f.path)
			}
		}
	}

	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create ZLAW_HOME: %w", err)
	}
	if err := os.MkdirAll(managerDir, 0o700); err != nil {
		return fmt.Errorf("create manager agent dir: %w", err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
	}

	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "Workspace created at %s\n", home)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  1. Add credentials:  zlaw auth add --provider anthropic")
	fmt.Fprintln(os.Stdout, "  2. Edit the manager: $EDITOR "+filepath.Join(managerDir, "agent.toml"))
	fmt.Fprintln(os.Stdout, "  3. Start the hub:    zlaw hub start")
	return nil
}

// initAgent creates a single named agent directory under $ZLAW_HOME/agents/<name>.
func (c *InitCmd) initAgent(name string) error {
	agentDir := filepath.Join(config.ZlawHome(), "agents", name)

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
		{filepath.Join(agentDir, "agent.toml"), fmt.Sprintf(agentTOMLTemplate, name), 0o600},
		{filepath.Join(agentDir, "SOUL.md"), soulMDTemplate, 0o644},
		{filepath.Join(agentDir, "IDENTITY.md"), fmt.Sprintf(identityMDTemplate, name), 0o644},
	}

	if !c.Force {
		for _, f := range files {
			if _, err := os.Stat(f.path); err == nil {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", f.path)
			}
		}
	}

	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
	}

	fmt.Fprintf(os.Stdout, "\nAgent %q created at %s\n", name, agentDir)
	fmt.Fprintf(os.Stdout, "Edit agent.toml to configure the LLM backend, then run:\n")
	fmt.Fprintf(os.Stdout, "  zlaw agent --agent %s serve\n", name)
	return nil
}
