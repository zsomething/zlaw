package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// BootstrapConfig holds configuration for bootstrapping a new zlaw home.
type BootstrapConfig struct {
	// Home is the zlaw home directory path.
	Home string
	// ManagerAgentDir is the directory for the default manager agent.
	// When empty, defaults to $ZLAW_HOME/agents/manager.
	ManagerAgentDir string
	// Force overwrites existing files if true.
	Force bool
}

// DefaultBootstrapConfig returns a BootstrapConfig for the default zlaw home.
func DefaultBootstrapConfig() BootstrapConfig {
	home := ZlawHome()
	return BootstrapConfig{
		Home:            home,
		ManagerAgentDir: filepath.Join(home, "agents", "manager"),
	}
}

// Exists checks if zlaw.toml already exists at the configured home.
func (c BootstrapConfig) Exists() bool {
	_, err := os.Stat(filepath.Join(c.Home, "zlaw.toml"))
	return err == nil
}

// CreateZlawHome creates the zlaw home structure: zlaw.toml, secrets.toml,
// and the agents/ directory. It does not create the manager agent files
// (those are created by CreateAgent).
func (c BootstrapConfig) CreateZlawHome() error {
	if !c.Force {
		if c.Exists() {
			return fmt.Errorf("zlaw home already exists at %s", c.Home)
		}
	}

	// Create home directory if needed.
	if err := os.MkdirAll(c.Home, 0o700); err != nil {
		return fmt.Errorf("create home directory: %w", err)
	}

	// Create zlaw.toml.
	zlawPath := filepath.Join(c.Home, "zlaw.toml")
	if err := c.writeZlawTOML(zlawPath); err != nil {
		return err
	}

	// Create secrets.toml.
	secretsPath := filepath.Join(c.Home, "secrets.toml")
	secretsContent := `# Secrets are managed by ctl. Agents receive values via env vars.
# Format: KEY = "value"
`
	if err := os.WriteFile(secretsPath, []byte(secretsContent), 0o600); err != nil {
		return fmt.Errorf("create secrets.toml: %w", err)
	}

	// Create agents/ directory.
	agentsDir := filepath.Join(c.Home, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("create agents directory: %w", err)
	}

	return nil
}

// writeZlawTOML writes the initial zlaw.toml content.
func (c BootstrapConfig) writeZlawTOML(path string) error {
	managerDir := c.ManagerAgentDir
	if managerDir == "" {
		managerDir = filepath.Join(c.Home, "agents", "manager")
	}

	content := fmt.Sprintf(zlawTOMLTemplate, managerDir)
	return os.WriteFile(path, []byte(content), 0o600)
}

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

// SetupAgentConfig holds configuration for creating a new agent.
type SetupAgentConfig struct {
	// ID is the agent identifier.
	ID string
	// Home is the zlaw home directory.
	Home string
	// Dir is the agent directory. When empty, defaults to $ZLAW_HOME/agents/<id>.
	Dir string
	// Force overwrites existing files if true.
	Force bool
}

// DefaultSetupAgentConfig returns an SetupAgentConfig for the given agent ID.
func DefaultSetupAgentConfig(id string) SetupAgentConfig {
	return SetupAgentConfig{
		ID:   id,
		Home: ZlawHome(),
	}
}

// AgentDir returns the agent's directory path.
func (c SetupAgentConfig) AgentDir() string {
	if c.Dir != "" {
		return c.Dir
	}
	return filepath.Join(c.Home, "agents", c.ID)
}

// Exists checks if the agent already exists (SOUL.md or IDENTITY.md present).
func (c SetupAgentConfig) Exists() bool {
	dir := c.AgentDir()
	_, err := os.Stat(filepath.Join(dir, "SOUL.md"))
	return err == nil
}

// CreateAgent creates the agent home directory with SOUL.md and IDENTITY.md.
func (c SetupAgentConfig) CreateAgent() error {
	dir := c.AgentDir()

	if !c.Force {
		if c.Exists() {
			return fmt.Errorf("agent %q already exists at %s", c.ID, dir)
		}
	}

	// Create directories.
	workspaceDir := filepath.Join(dir, "workspace")
	skillsDir := filepath.Join(dir, "skills")

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("create skills dir: %w", err)
	}

	// Create SOUL.md.
	soulPath := filepath.Join(dir, "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(soulMDTemplate), 0o644); err != nil {
		return fmt.Errorf("write SOUL.md: %w", err)
	}

	// Create IDENTITY.md.
	identityPath := filepath.Join(dir, "IDENTITY.md")
	identityContent := fmt.Sprintf(identityMDTemplate, c.ID)
	if err := os.WriteFile(identityPath, []byte(identityContent), 0o644); err != nil {
		return fmt.Errorf("write IDENTITY.md: %w", err)
	}

	return nil
}

// soulMDTemplate is the default SOUL.md content.
const soulMDTemplate = `You are a helpful personal assistant.
`

// identityMDTemplate has the agent name substituted for %s.
const identityMDTemplate = `# Identity

Your name is %s.
`
