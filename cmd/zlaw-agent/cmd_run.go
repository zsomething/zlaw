package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"

	"os"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

func runRun(ctx context.Context, args []string, agentDir string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	sessionID := fs.String("session", "default", "session identifier")
	verbose := fs.Bool("verbose", false, "show thinking and tool calls")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if agentDir == "" {
		return fmt.Errorf("--agent-dir is required (or set ZLAW_AGENT_DIR)")
	}

	// --- Load config ---
	loader, err := config.NewLoader(agentDir, nil, logger)
	if err != nil {
		return fmt.Errorf("create config loader: %w", err)
	}
	cfg, personality, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Build LLM client ---
	llmClient, err := llm.NewClientFromConfig(cfg.LLM, "", logger)
	if err != nil {
		return fmt.Errorf("create llm client: %w", err)
	}

	// --- Build tool registry ---
	registry := tools.NewRegistry()
	registry.Register(builtin.CurrentTime{})
	registry.Register(builtin.ReadFile{})
	registry.Register(builtin.WriteFile{})
	registry.Register(builtin.EditFile{})
	registry.Register(builtin.Glob{})
	registry.Register(builtin.GrepFiles{})
	registry.Register(builtin.Bash{})

	// --- Build agent ---
	history := agent.NewHistory()
	ag := agent.New(cfg.Agent.Name, llmClient, registry, history, logger)

	// --- Build system prompt ---
	systemPrompt := agent.BuildSystemPrompt(personality)

	// --- Start config hot-reload ---
	go func() {
		if err := loader.Watch(ctx); err != nil {
			logger.Error("config watcher stopped", "err", err)
		}
	}()

	// --- Build CLI adapter ---
	adapter := cli.New(ag, systemPrompt, *verbose, nil, nil, logger)

	if cli.IsTerminal(os.Stdin) {
		return adapter.RunInteractive(ctx, *sessionID)
	}
	return adapter.RunStdin(ctx, *sessionID)
}
