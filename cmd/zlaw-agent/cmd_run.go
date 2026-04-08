package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"

	"os"

	"github.com/chickenzord/zlaw/internal/adapters/cli"
	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

// buildHistory returns a durable History backed by a JSONLFileStore, falling
// back to in-memory if the session directory cannot be created.
func buildHistory(agentName, channel string, logger *slog.Logger) (*agent.History, error) {
	dir, err := agent.SessionDir(agentName)
	if err != nil {
		logger.Warn("cannot resolve session dir, using in-memory history", "error", err)
		return agent.NewHistory(), nil
	}
	store := agent.NewJSONLFileStore(dir)
	logger.Info("session history", "dir", dir)
	return agent.NewHistoryWithStore(store, channel), nil
}

// resolveAgentDir returns the effective agent directory.
// agentDir (--agent-dir) takes precedence; agentName (--agent) falls back to
// $ZLAW_HOME/agents/<name>; both empty is an error.
func resolveAgentDir(agentName, agentDir string) (string, error) {
	if agentDir != "" {
		return agentDir, nil
	}
	if agentName != "" {
		return filepath.Join(config.ZlawHome(), "agents", agentName), nil
	}
	return "", fmt.Errorf("--agent <name> or --agent-dir <path> is required (or set ZLAW_AGENT / ZLAW_AGENT_DIR)")
}

func runRun(ctx context.Context, args []string, agentName, agentDir string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	sessionID := fs.String("session", "default", "session identifier")
	verbose := fs.Bool("verbose", false, "show thinking and tool calls")
	showUsage := fs.Bool("show-usage", false, "print token usage after each turn (per-turn and cumulative)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedDir, err := resolveAgentDir(agentName, agentDir)
	if err != nil {
		return err
	}
	agentDir = resolvedDir

	// --- Load config ---
	var promptPtr atomic.Pointer[string]
	onChange := func(_ config.AgentConfig, p config.Personality) {
		s := agent.BuildSystemPrompt(p)
		promptPtr.Store(&s)
		logger.Info("system prompt reloaded")
	}
	loader, err := config.NewLoader(agentDir, onChange, logger)
	if err != nil {
		return fmt.Errorf("create config loader: %w", err)
	}
	cfg, personality, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Seed the atomic with the initial prompt.
	initial := agent.BuildSystemPrompt(personality)
	promptPtr.Store(&initial)

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
	history, err := buildHistory(cfg.Agent.Name, "cli", logger)
	if err != nil {
		return fmt.Errorf("create session history: %w", err)
	}
	ag := agent.New(cfg.Agent.Name, llmClient, registry, history, logger)
	if cfg.LLM.ContextTokenBudget > 0 {
		var summarizer agent.Summarizer
		if cfg.LLM.ContextSummarizeThreshold > 0 {
			summarizer = agent.NewLLMSummarizer(llmClient)
		}
		opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
			TokenBudget:        cfg.LLM.ContextTokenBudget,
			SummarizeThreshold: cfg.LLM.ContextSummarizeThreshold,
			SummarizeTurns:     cfg.LLM.ContextSummarizeTurns,
		}, summarizer, logger)
		ag.SetContextOptimizer(opt)
	}

	// --- Start config hot-reload ---
	go func() {
		if err := loader.Watch(ctx); err != nil {
			logger.Error("config watcher stopped", "err", err)
		}
	}()

	// --- Build CLI adapter ---
	adapter := cli.New(ag, func() string { return *promptPtr.Load() }, *verbose, nil, nil, logger)
	adapter.SetShowUsage(*showUsage)

	if cli.IsTerminal(os.Stdin) {
		return adapter.RunInteractive(ctx, *sessionID)
	}
	return adapter.RunStdin(ctx, *sessionID)
}
