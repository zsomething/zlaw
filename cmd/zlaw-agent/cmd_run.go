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
	"github.com/chickenzord/zlaw/internal/skills"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
)

// buildMemoryStore returns a MarkdownFileStore for the agent's long-term
// memories. Falls back to nil on error (caller should skip memory tools).
func buildMemoryStore(agentName string, logger *slog.Logger) agent.MemoryStore {
	dir, err := agent.MemoryDir(agentName)
	if err != nil {
		logger.Warn("cannot resolve memory dir, memory tools disabled", "error", err)
		return nil
	}
	logger.Info("memory store", "dir", dir)
	return agent.NewMarkdownFileStore(dir)
}

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
	// stickyBlocks are fixed at startup (content lives in Go source).
	// The hot-reload callback only needs to rebuild the personality portion.
	var stickyBlocks []agent.StickyBlock
	// (blocks will be populated after cfg is loaded below)

	onChange := func(_ config.AgentConfig, p config.Personality) {
		s := agent.BuildSystemPrompt(nil, p)
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

	// Collect enabled sticky blocks from config.
	// Each flag enables one named block whose content lives in Go source,
	// forming a stable cache prefix unaffected by hot-reloads of user files.
	if cfg.Sticky.ProactiveMemorySave {
		stickyBlocks = append(stickyBlocks, agent.StickyBlock{
			Name:    "memory-behavior",
			Content: agent.StickyProactiveMemorySave,
		})
		logger.Info("sticky block enabled", "name", "memory-behavior")
	}

	// Seed the atomic with the personality-only prompt.
	// The agent combines it with stickyBlocks at call time.
	initial := agent.BuildSystemPrompt(nil, personality)
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
	registry.Register(builtin.WebFetch{})
	registry.Register(builtin.WebSearch{})
	registry.Register(builtin.HTTPRequest{})
	registry.Register(builtin.Configure{Loader: loader})
	memStore := buildMemoryStore(cfg.Agent.Name, logger)
	if memStore != nil {
		registry.Register(builtin.MemorySave{Store: memStore})
		registry.Register(builtin.MemoryRecall{Store: memStore})
		registry.Register(builtin.MemoryDelete{Store: memStore})
	}

	// --- Discover skills ---
	discoveredSkills, err := skills.Discover(config.ZlawHome(), cfg.Agent.Name, logger)
	if err != nil {
		logger.Warn("skill discovery failed, continuing without skills", "error", err)
		discoveredSkills = nil
	}
	var skillsMap map[string]skills.Skill
	if len(discoveredSkills) > 0 {
		skillsMap = make(map[string]skills.Skill, len(discoveredSkills))
		for _, s := range discoveredSkills {
			skillsMap[s.Name] = s
		}
		registry.Register(builtin.SkillLoad{Skills: skillsMap})
		logger.Info("skills discovered", "count", len(discoveredSkills))
	}

	if len(cfg.Tools.Allowed) > 0 {
		registry.SetAllowlist(cfg.Tools.Allowed)
		logger.Info("tool allowlist enforced", "allowed", cfg.Tools.Allowed)
	}
	registry.SetMaxResultBytes(cfg.Tools.MaxResultBytes)

	// --- Build agent ---
	history, err := buildHistory(cfg.Agent.Name, "cli", logger)
	if err != nil {
		return fmt.Errorf("create session history: %w", err)
	}
	ag := agent.New(cfg.Agent.Name, llmClient, registry, history, logger)
	ag.SetStickyBlocks(stickyBlocks)
	if len(discoveredSkills) > 0 {
		ag.SetSkillsSection(agent.BuildSkillsSection(discoveredSkills))
	}
	if memStore != nil {
		ag.SetMemoryStore(memStore, cfg.LLM.MaxMemoryTokens)
	}
	if cfg.LLM.ContextTokenBudget > 0 {
		var summarizer agent.Summarizer
		if cfg.LLM.ContextSummarizeThreshold > 0 {
			summarizerClient := llmClient
			if cfg.LLM.ContextSummarizeModel != "" {
				summarizeCfg := cfg.LLM
				summarizeCfg.Model = cfg.LLM.ContextSummarizeModel
				sc, err := llm.NewClientFromConfig(summarizeCfg, "", logger)
				if err != nil {
					return fmt.Errorf("create summarizer llm client: %w", err)
				}
				summarizerClient = sc
				logger.Info("summarizer using separate model", "model", cfg.LLM.ContextSummarizeModel)
			}
			summarizer = agent.NewLLMSummarizer(summarizerClient)
		}
		var pruneLevels []agent.PruneLevel
		for _, s := range cfg.LLM.ContextPruneLevels {
			pruneLevels = append(pruneLevels, agent.PruneLevel(s))
		}
		opt := agent.NewContextOptimizer(agent.ContextOptimizerConfig{
			TokenBudget:        cfg.LLM.ContextTokenBudget,
			SummarizeThreshold: cfg.LLM.ContextSummarizeThreshold,
			SummarizeTurns:     cfg.LLM.ContextSummarizeTurns,
			PruneLevels:        pruneLevels,
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
	adapter.SetHistoryManager(history)
	adapter.SetShowUsage(*showUsage)
	if skillsMap != nil {
		adapter.SetSkillLoader(func(name string) (string, error) {
			s, ok := skillsMap[name]
			if !ok {
				return "", fmt.Errorf("skill %q not found", name)
			}
			return s.Body, nil
		})
	}
	if len(cfg.Context.Prefill) > 0 {
		adapter.SetPrefill(func() (string, error) {
			return agent.BuildPrefill(agentDir, cfg.Context.Prefill)
		})
	}

	if cli.IsTerminal(os.Stdin) {
		return adapter.RunInteractive(ctx, *sessionID)
	}
	return adapter.RunStdin(ctx, *sessionID)
}
