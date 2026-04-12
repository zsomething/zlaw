package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chickenzord/zlaw/internal/config"
)

const agentTOMLTemplate = `[agent]
name = %q
description = ""

[llm]
backend = "minimax"
model = "MiniMax-M2.7"
auth_profile = "minimax"
max_tokens = 4096
timeout_sec = 60

# Semantic (embedding-based) memory search. When backend is set, memory_recall
# uses vector similarity instead of keyword matching. Remove this section to
# fall back to keyword search.
#
# [memory.embedder]
# backend      = "minimax-openai"      # OpenAI-compat endpoint for embeddings
# model        = "embo-01"
# auth_profile = "minimax"             # same credentials as the LLM backend

[tools]
# allowed = ["bash", "read_file", ...]  # uncomment to restrict tool access

[adapter]
type = "cli"
`

const soulMDTemplate = `You are a helpful personal assistant.
`

const identityMDTemplate = `# Identity

Your name is %s.
`

// runInit bootstraps a new agent directory under $ZLAW_HOME/agents/<name>.
// It writes agent.toml, SOUL.md, and IDENTITY.md with starter content.
// Fails if any file already exists unless --force is given.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	name := fs.String("name", "", "agent name (required); directory created at $ZLAW_HOME/agents/<name>")
	force := fs.Bool("force", false, "overwrite existing files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *name == "" {
		fs.Usage()
		return fmt.Errorf("--name is required")
	}

	agentDir := filepath.Join(config.ZlawHome(), "agents", *name)

	files := map[string]string{
		"agent.toml":   fmt.Sprintf(agentTOMLTemplate, *name),
		"SOUL.md":      soulMDTemplate,
		"IDENTITY.md":  fmt.Sprintf(identityMDTemplate, *name),
	}

	// Check for existing files before writing anything.
	if !*force {
		for filename := range files {
			path := filepath.Join(agentDir, filename)
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("file already exists: %s (use --force to overwrite)", path)
			}
		}
	}

	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	for filename, content := range files {
		path := filepath.Join(agentDir, filename)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
		fmt.Fprintf(os.Stdout, "  created %s\n", path)
	}

	fmt.Fprintf(os.Stdout, "\nAgent %q initialised at %s\n", *name, agentDir)
	fmt.Fprintf(os.Stdout, "Edit agent.toml to configure the LLM backend, then run:\n")
	fmt.Fprintf(os.Stdout, "  zlaw-agent --agent %s serve\n", *name)
	return nil
}
