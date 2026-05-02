package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
)

// ── Templates ────────────────────────────────────────────────────────────────

// secretsTOMLTemplate is the initial secrets.toml content.
const secretsTOMLTemplate = `# Secrets are managed by ctl. Agents receive values via env vars.
# Format: KEY = "value"
# Example:
# MINIMAX_API_KEY = "sk-..."
# ANTHROPIC_API_KEY = "sk-ant-..."
`

// zlawTOMLTemplate has the absolute agent dir substituted for %s.
const zlawTOMLTemplate = `[hub]
name = "main"
description = "zlaw hub"

[[agents]]
id = "manager"
dir = %q
executor = "subprocess"
target = "local"
restart_policy = "on-failure"

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
// manager agent home directory (SOUL.md, IDENTITY.md, workspace/).
func (c *InitCmd) initWorkspace() error {
	home := config.ZlawHome()
	managerDir := filepath.Join(home, "agents", "manager")
	managerWorkspace := filepath.Join(managerDir, "workspace")

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
		{filepath.Join(home, "zlaw.toml"), fmt.Sprintf(zlawTOMLTemplate, managerDir), 0o600},
		{filepath.Join(home, "secrets.toml"), secretsTOMLTemplate, 0o600},
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

	if err := os.MkdirAll(managerDir, 0o700); err != nil {
		return fmt.Errorf("create manager agent dir: %w", err)
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
	fmt.Fprintf(os.Stdout, "Initialized at %s\n", home)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  1. Add credentials:  edit", managerDir+"/credentials.toml")
	fmt.Fprintln(os.Stdout, "  2. Start the hub:    zlaw hub start")
	return nil
}

// initAgent creates a single agent home under $ZLAW_HOME/agents/<name>.
// Scaffolds SOUL.md, IDENTITY.md, and workspace/ in the agent home.
func (c *InitCmd) initAgent(name string) error {
	agentDir := filepath.Join(config.ZlawHome(), "agents", name)
	agentWorkspace := filepath.Join(agentDir, "workspace")

	type fileEntry struct {
		path    string
		content string
		mode    os.FileMode
	}

	files := []fileEntry{
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
	if err := os.MkdirAll(agentWorkspace, 0o700); err != nil {
		return fmt.Errorf("create agent workspace: %w", err)
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), f.mode); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
		fmt.Fprintf(os.Stdout, "  created  %s\n", f.path)
	}

	fmt.Fprintf(os.Stdout, "\nAgent %q created at %s\n", name, agentDir)
	fmt.Fprintf(os.Stdout, "Register with hub by adding to zlaw.toml:\n")
	fmt.Fprintf(os.Stdout, "  [[agents]]\n  id = %q\n  dir = %q\n", name, agentDir)
	fmt.Fprintf(os.Stdout, "Then run: zlaw hub start\n")
	return nil
}
