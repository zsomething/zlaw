package hub

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
)

// resolveAgentDir returns the agent directory for entry.
// Falls back to $ZLAW_HOME/agents/<name> when entry.Dir is empty.
func resolveAgentDir(entry config.AgentEntry) string {
	if entry.Dir != "" {
		return entry.Dir
	}
	return filepath.Join(config.ZlawHome(), "agents", entry.Name)
}

// resolveWorkspaceDir returns the workspace directory for entry.
// Falls back to $ZLAW_HOME/workspaces/<name> when entry.Workspace is empty.
func resolveWorkspaceDir(entry config.AgentEntry) string {
	if entry.Workspace != "" {
		return entry.Workspace
	}
	return filepath.Join(config.ZlawHome(), "workspaces", entry.Name)
}

// BuildCredentialEnv reads the agent's agent.toml to discover required auth
// profiles (LLM + memory embedder + adapters), validates them against the
// user-maintained credentials file (agents/<name>/credentials.toml),
// expands ${VAR} env-var references, and writes a runtime-only copy to
// agents/<name>/credentials.active.toml which is injected to the agent.
//
// The source file (credentials.toml) is never modified by the hub.
// The active file (credentials.active.toml) is regenerated on every spawn.
//
// If agent.toml references no auth profiles the function is a no-op and
// returns nil. If a referenced profile is absent from the source store,
// an error naming the missing profile is returned and the caller must abort.
func BuildCredentialEnv(entry config.AgentEntry) ([]string, error) {
	agentDir := resolveAgentDir(entry)

	agentTOMLPath := filepath.Join(agentDir, "agent.toml")
	if _, statErr := os.Stat(agentTOMLPath); os.IsNotExist(statErr) {
		// No agent.toml (custom binary with no config): skip credential injection.
		return nil, nil
	}
	agentCfg, err := config.LoadAgentConfigFile(agentTOMLPath)
	if err != nil {
		return nil, fmt.Errorf("read agent config: %w", err)
	}

	profiles := collectAuthProfiles(agentCfg)
	if len(profiles) == 0 {
		return nil, nil
	}

	// Read from the user-maintained source file (credentials.toml).
	sourceCredsPath := filepath.Join(agentDir, "credentials.toml")
	store, err := credentials.LoadStore(sourceCredsPath)
	if err != nil {
		return nil, fmt.Errorf("load credentials from %s: %w", sourceCredsPath, err)
	}

	// Validate every required profile exists; collect only what's needed.
	needed := credentials.CredentialStore{Profiles: make(map[string]credentials.CredentialProfile, len(profiles))}
	for _, name := range profiles {
		profile, ok := store.Profiles[name]
		if !ok {
			return nil, fmt.Errorf("agent %q requires auth profile %q which does not exist in %s", entry.Name, name, sourceCredsPath)
		}
		needed.Profiles[name] = profile
	}

	// Write the expanded credentials to the runtime-active file.
	// This file is owned by the hub and regenerated on every agent spawn.
	activeCredsPath := filepath.Join(agentDir, "credentials.active.toml")
	if err := credentials.SaveStore(activeCredsPath, needed); err != nil {
		return nil, fmt.Errorf("write credentials.active.toml: %w", err)
	}

	return []string{"ZLAW_CREDENTIALS_FILE=" + activeCredsPath}, nil
}

// collectAuthProfiles returns the unique, non-empty auth profile names
// referenced by cfg (LLM + memory embedder + adapters).
func collectAuthProfiles(cfg config.AgentConfig) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	add(cfg.LLM.AuthProfile)
	add(cfg.Memory.Embedder.AuthProfile)

	// Collect adapter auth profiles (multi-adapter support).
	for _, adapter := range cfg.Adapter {
		add(adapter.AuthProfile)
	}

	return out
}
