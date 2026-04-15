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
// per-agent credentials file at agents/<name>/credentials.toml,
// and returns the env var to inject (ZLAW_CREDENTIALS_FILE=<path>).
//
// The per-agent credentials file is owned by the hub. The agent never has
// access to it at rest — only through this injected env var at runtime.
//
// If agent.toml references no auth profiles the function is a no-op and
// returns nil. If a referenced profile is absent from the store, an error
// naming the missing profile is returned and the caller must abort the spawn.
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

	// Per-agent credentials file.
	credsPath := filepath.Join(agentDir, "credentials.toml")

	store, err := credentials.LoadStore(credsPath)
	if err != nil {
		return nil, fmt.Errorf("load credentials store: %w", err)
	}

	// Validate every required profile exists; collect only what's needed.
	needed := credentials.CredentialStore{Profiles: make(map[string]credentials.CredentialProfile, len(profiles))}
	for _, name := range profiles {
		profile, ok := store.Profiles[name]
		if !ok {
			return nil, fmt.Errorf("agent %q requires auth profile %q which does not exist in %s", entry.Name, name, credsPath)
		}
		needed.Profiles[name] = profile
	}

	// Write the validated credentials (hub owns this file, agent gets via env var).
	if err := credentials.SaveStore(credsPath, needed); err != nil {
		return nil, fmt.Errorf("write agent credentials file: %w", err)
	}

	return []string{"ZLAW_CREDENTIALS_FILE=" + credsPath}, nil
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
	for _, adapter := range cfg.Adapter.Adapters {
		add(adapter.AuthProfile)
	}

	return out
}
