package main

import (
	"fmt"
	"os"

	"github.com/zsomething/zlaw/internal/config"
)

// InitCmd bootstraps $ZLAW_HOME or create a named agent workspace.
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

// initWorkspace creates the full $ZLAW_HOME layout using config package.
func (c *InitCmd) initWorkspace() error {
	cfg := config.DefaultBootstrapConfig()
	cfg.Force = c.Force

	if err := cfg.CreateZlawHome(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// Create the manager agent.
	agentCfg := config.DefaultSetupAgentConfig("manager")
	agentCfg.Force = c.Force
	if err := agentCfg.CreateAgent(); err != nil {
		return fmt.Errorf("create manager agent: %w", err)
	}

	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "Initialized at %s\n", cfg.Home)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Next steps:")
	fmt.Fprintln(os.Stdout, "  1. Add credentials:  edit", cfg.Home+"/secrets.toml")
	fmt.Fprintln(os.Stdout, "  2. Start the hub:    zlaw hub start")
	return nil
}

// initAgent creates a single agent home using config package.
func (c *InitCmd) initAgent(name string) error {
	cfg := config.DefaultSetupAgentConfig(name)
	cfg.Force = c.Force

	if err := cfg.CreateAgent(); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	agentDir := cfg.AgentDir()
	fmt.Fprintf(os.Stdout, "\nAgent %q created at %s\n", name, agentDir)
	fmt.Fprintf(os.Stdout, "Register with hub by adding to zlaw.toml:\n")
	fmt.Fprintf(os.Stdout, "  [[agents]]\n  id = %q\n  dir = %q\n", name, agentDir)
	fmt.Fprintf(os.Stdout, "Then run: zlaw hub start\n")
	return nil
}
