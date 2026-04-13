package hub

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm/auth"
)

// resolveAgentDir returns the agent directory for entry.
// Falls back to $ZLAW_HOME/agents/<name> when entry.Dir is empty.
func resolveAgentDir(entry config.AgentEntry) string {
	if entry.Dir != "" {
		return entry.Dir
	}
	return filepath.Join(config.ZlawHome(), "agents", entry.Name)
}

// BuildCredentialEnv reads the agent's agent.toml to discover required auth
// profiles, validates them against the credential store at credentialsPath,
// writes a minimal per-agent credentials file under $ZLAW_HOME/run/, and
// returns the env vars to inject (ZLAW_CREDENTIALS_FILE=<path>).
//
// If agent.toml references no auth profiles the function is a no-op and
// returns nil. If a referenced profile is absent from the store, an error
// naming the missing profile is returned and the caller must abort the spawn.
func BuildCredentialEnv(entry config.AgentEntry, credentialsPath string) ([]string, error) {
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

	if credentialsPath == "" {
		credentialsPath = auth.DefaultCredentialsPath()
	}

	store, err := auth.LoadStore(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("load credentials store: %w", err)
	}

	// Validate every required profile exists; collect only what's needed.
	needed := auth.CredentialStore{Profiles: make(map[string]auth.CredentialProfile, len(profiles))}
	for _, name := range profiles {
		profile, ok := store.Profiles[name]
		if !ok {
			return nil, fmt.Errorf("agent %q requires auth profile %q which does not exist in credentials.toml", entry.Name, name)
		}
		needed.Profiles[name] = profile
	}

	// Write a minimal per-agent credentials file.
	runDir := filepath.Join(config.ZlawHome(), "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	outPath := filepath.Join(runDir, "hub-creds-"+entry.Name+".toml")
	if err := auth.SaveStore(outPath, needed); err != nil {
		return nil, fmt.Errorf("write agent credentials file: %w", err)
	}

	return []string{"ZLAW_CREDENTIALS_FILE=" + outPath}, nil
}

// collectAuthProfiles returns the unique, non-empty auth profile names
// referenced by cfg (LLM + memory embedder).
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
	return out
}
