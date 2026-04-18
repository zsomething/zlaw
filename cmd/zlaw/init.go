package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

// ── Templates ────────────────────────────────────────────────────────────────

const zlawTOMLTemplate = `[hub]
name = "main"
description = "zlaw hub"
audit_log_path = ".zlaw/audit.log"

# Each agent supervised by this hub.
# The hub scaffolds agent directories automatically.
[[agents]]
name = "manager"
manager = true

[web]
# Web dashboard listen address. Requires [web] enabled = true.
# bind_address = "127.0.0.1:7420"

[nats]
# Embedded NATS server listen address. Defaults to 127.0.0.1:4222.
# listen = "127.0.0.1:4222"
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
// Without --agent: full workspace bootstrap (zlaw.toml, workspaces/manager/).
//
// With --agent <name>: create a named agent only (no hub config).
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

// initWorkspace creates the full $ZLAW_HOME layout: zlaw.toml and the
// manager agent workspace. The hub will scaffold agent directories
// (agent.toml, credentials.toml) at startup.
func (c *InitCmd) initWorkspace() error {
	home := config.ZlawHome()
	managerWorkspace := filepath.Join(home, "workspaces", "manager")

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
		{filepath.Join(home, "zlaw.toml"), zlawTOMLTemplate, 0o600},
		{filepath.Join(managerWorkspace, "SOUL.md"), soulMDTemplate, 0o644},
		{filepath.Join(managerWorkspace, "IDENTITY.md"), fmt.Sprintf(identityMDTemplate, "Manager"), 0o644},
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
	if err := os.MkdirAll(managerWorkspace, 0o700); err != nil {
		return fmt.Errorf("create manager workspace: %w", err)
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
	fmt.Fprintln(os.Stdout, "  1. Start the hub:    zlaw hub start")
	fmt.Fprintln(os.Stdout, "     The hub will scaffold agent dirs and prompt for credentials.")
	fmt.Fprintln(os.Stdout, "  2. Or set credentials manually:")
	fmt.Fprintln(os.Stdout, "       zlaw hub auth set --agent manager --profile anthropic --key $ANTHROPIC_API_KEY")
	return nil
}

// initAgent creates a single agent workspace under $ZLAW_HOME/workspaces/<name>.
// The hub will scaffold the agent directory (agent.toml, credentials.toml)
// when it starts or when you run 'zlaw hub agent create <name>'.
func (c *InitCmd) initAgent(name string) error {
	workspaceDir := filepath.Join(config.ZlawHome(), "workspaces", name)

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
		{filepath.Join(workspaceDir, "SOUL.md"), soulMDTemplate, 0o644},
		{filepath.Join(workspaceDir, "IDENTITY.md"), fmt.Sprintf(identityMDTemplate, name), 0o644},
	}

	if !c.Force {
		for _, f := range files {
			if _, err := os.Stat(f.path); err == nil {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", f.path)
			}
		}
	}

	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
	}

	fmt.Fprintf(os.Stdout, "\nWorkspace %q created at %s\n", name, workspaceDir)
	fmt.Fprintf(os.Stdout, "Add to zlaw.toml to register with hub:\n")
	fmt.Fprintf(os.Stdout, "  [[agents]]\n  name = %q\n", name)
	fmt.Fprintf(os.Stdout, "Then run: zlaw hub start\n")
	return nil
}
