package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm/auth"
)

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

const credentialsTemplate = `# Credential profiles for zlaw-hub and its agents.
# Add profiles with: zlaw-hub auth add --provider <name>

[profiles]
`

const managerAgentTOMLTemplate = `[agent]
name = "manager"
description = "Manager agent — receives user input and delegates to peers."
manager = true

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

const managerSoulMDTemplate = `You are a capable personal assistant and the designated manager agent for this zlaw hub.
You receive user requests, handle them directly when possible, and delegate specialised tasks to peer agents.
You are thoughtful, concise, and reliable.
`

const managerIdentityMDTemplate = `# Identity

Your name is Manager.
You are the primary interface between the user and the zlaw multi-agent platform.
`

// runInit scaffolds $ZLAW_HOME for hub operation:
//   - zlaw.toml skeleton
//   - credentials.toml (0600 template)
//   - agents/manager/ with agent.toml, SOUL.md, IDENTITY.md
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite existing files")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: zlaw-hub init [--force]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Scaffolds $ZLAW_HOME with a hub config, credentials file,")
		fmt.Fprintln(os.Stderr, "and a manager agent directory.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

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
		{filepath.Join(managerDir, "agent.toml"), managerAgentTOMLTemplate, 0o600},
		{filepath.Join(managerDir, "SOUL.md"), managerSoulMDTemplate, 0o644},
		{filepath.Join(managerDir, "IDENTITY.md"), managerIdentityMDTemplate, 0o644},
	}

	// Pre-flight: check for conflicts before writing anything.
	if !*force {
		for _, f := range files {
			if _, err := os.Stat(f.path); err == nil {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", f.path)
			}
		}
	}

	// Create directories.
	if err := os.MkdirAll(home, 0o700); err != nil {
		return fmt.Errorf("create ZLAW_HOME: %w", err)
	}
	if err := os.MkdirAll(managerDir, 0o700); err != nil {
		return fmt.Errorf("create manager agent dir: %w", err)
	}

	// Write files.
	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
	}

	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "Hub scaffold created at %s\n", home)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  1. Add credentials:  zlaw-hub auth add --provider anthropic")
	fmt.Fprintln(os.Stdout, "  2. Edit the manager: $EDITOR "+filepath.Join(managerDir, "agent.toml"))
	fmt.Fprintln(os.Stdout, "  3. Start the hub:    zlaw-hub start")
	return nil
}
