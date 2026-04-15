package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"

	"github.com/zsomething/zlaw/internal/adapters/cli"
	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/skills"
	"github.com/zsomething/zlaw/internal/tools"
	"github.com/zsomething/zlaw/internal/tools/builtin"
)

// AgentRunOptions holds options for running an agent in interactive / stdin mode.
type AgentRunOptions struct {
	Session   string
	Verbose   bool
	ShowUsage bool
}

// RunAgent wires up an agent from agentDir and workspace, and runs it
// in interactive or stdin mode.
func RunAgent(ctx context.Context, agentDir string, workspaceDir string, opts AgentRunOptions, logger *slog.Logger) error {
	var promptPtr atomic.Pointer[string]
	var stickyBlocks []agent.StickyBlock

	onChange := func(_ config.AgentConfig, p config.Personality) {
		s := agent.BuildSystemPrompt(nil, p)
		promptPtr.Store(&s)
		logger.Info("system prompt reloaded")
	}
	loader, err := config.NewLoader(agentDir, workspaceDir, onChange, logger)
	if err != nil {
		return fmt.Errorf("create config loader: %w", err)
	}
	cfg, personality, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Sticky.ProactiveMemorySave {
		stickyBlocks = append(stickyBlocks, agent.StickyBlock{
			Name:    "memory-behavior",
			Content: agent.StickyProactiveMemorySave,
		})
		logger.Info("sticky block enabled", "name", "memory-behavior")
	}

	initial := agent.BuildSystemPrompt(nil, personality)
	promptPtr.Store(&initial)

	llmClient, err := llm.NewClientFromConfig(cfg.LLM, "", logger)
	if err != nil {
		return fmt.Errorf("create llm client: %w", err)
	}

	registry, _ := buildToolRegistry(ctx, cfg, loader, logger)

	discoveredSkills, err := skills.Discover(config.ZlawHome(), cfg.Agent.ID, logger)
	if err != nil {
		logger.Warn("skill discovery failed, continuing without skills", "error", err)
		discoveredSkills = nil
	}
	skillsMap := indexSkills(discoveredSkills, registry, logger)

	applyToolConfig(cfg, registry, logger)

	history, err := buildHistory(cfg.Agent.ID, "cli", logger)
	if err != nil {
		return fmt.Errorf("create session history: %w", err)
	}

	ag := buildAgent(ctx, cfg, llmClient, registry, history, stickyBlocks, discoveredSkills, logger)

	go func() {
		if err := loader.Watch(ctx); err != nil {
			logger.Error("config watcher stopped", "err", err)
		}
	}()

	adapter := cli.New(ag, func() string { return *promptPtr.Load() }, opts.Verbose, nil, nil, logger)
	adapter.SetHistoryManager(history)
	adapter.SetShowUsage(opts.ShowUsage)
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
		return adapter.RunInteractive(ctx, opts.Session)
	}
	return adapter.RunStdin(ctx, opts.Session)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func buildToolRegistry(ctx context.Context, cfg config.AgentConfig, loader *config.Loader, logger *slog.Logger) (*tools.Registry, *builtin.AgentDelegate) {
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

	// agent_delegate is always registered; Messenger/Registry are set later
	// when the hub connection is established (nil = standalone-mode error).
	delegateTool := &builtin.AgentDelegate{AgentID: cfg.Agent.ID}
	registry.Register(delegateTool)

	// Agent discovery tools — read-only, available to all agents.
	registry.Register(&builtin.ListAgents{Registry: builtin.NewAgentRegistry(nil)})
	registry.Register(&builtin.GetAgent{Registry: builtin.NewAgentRegistry(nil)})

	memStore := buildMemoryStore(ctx, cfg, "", logger)
	if memStore != nil {
		registry.Register(builtin.MemorySave{Store: memStore})
		registry.Register(builtin.MemoryRecall{Store: memStore})
		registry.Register(builtin.MemoryDelete{Store: memStore})
	}
	return registry, delegateTool
}

func indexSkills(discovered []skills.Skill, registry *tools.Registry, logger *slog.Logger) map[string]skills.Skill {
	if len(discovered) == 0 {
		return nil
	}
	m := make(map[string]skills.Skill, len(discovered))
	for _, s := range discovered {
		m[s.Name] = s
	}
	registry.Register(builtin.SkillLoad{Skills: m})
	logger.Info("skills discovered", "count", len(discovered))
	return m
}

func applyToolConfig(cfg config.AgentConfig, registry *tools.Registry, logger *slog.Logger) {
	if len(cfg.Tools.Allowed) > 0 {
		registry.SetAllowlist(cfg.Tools.Allowed)
		logger.Info("tool allowlist enforced", "allowed", cfg.Tools.Allowed)
	}
	registry.SetMaxResultBytes(cfg.Tools.MaxResultBytes)
}

func buildAgent(
	ctx context.Context,
	cfg config.AgentConfig,
	llmClient llm.Client,
	registry *tools.Registry,
	history *agent.History,
	stickyBlocks []agent.StickyBlock,
	discoveredSkills []skills.Skill,
	logger *slog.Logger,
) *agent.Agent {
	ag := agent.New(cfg.Agent.ID, llmClient, registry, history, logger)
	ag.SetStickyBlocks(stickyBlocks)
	if len(discoveredSkills) > 0 {
		ag.SetSkillsSection(agent.BuildSkillsSection(discoveredSkills))
	}

	memStore := buildMemoryStore(ctx, cfg, "", logger)
	if memStore != nil {
		ag.SetMemoryStore(memStore, cfg.LLM.MaxMemoryTokens)
	}

	if cfg.LLM.ContextTokenBudget > 0 {
		opt := buildContextOptimizer(cfg, llmClient, logger)
		ag.SetContextOptimizer(opt)
	}

	return ag
}

func buildMemoryStore(ctx context.Context, cfg config.AgentConfig, credPath string, logger *slog.Logger) agent.MemoryStore {
	dir, err := agent.MemoryDir(cfg.Agent.ID)
	if err != nil {
		logger.Warn("cannot resolve memory dir, memory tools disabled", "error", err)
		return nil
	}
	logger.Info("memory store", "dir", dir)

	if cfg.Memory.Embedder.Backend != "" {
		emb := cfg.Memory.Embedder
		if emb.AuthProfile == "" {
			emb.AuthProfile = cfg.LLM.AuthProfile
		}
		embedFunc, err := agent.NewEmbeddingFunc(emb, credPath) //nolint:contextcheck // NewEmbeddingFunc does not take a context
		if err != nil {
			logger.Warn("failed to build embedder, falling back to keyword search", "error", err)
			return agent.NewMarkdownFileStore(dir)
		}
		store, err := agent.NewSemanticMemoryStore(ctx, dir, embedFunc, logger)
		if err != nil {
			logger.Warn("failed to build semantic memory store, falling back to keyword search", "error", err)
			return agent.NewMarkdownFileStore(dir)
		}
		logger.Info("semantic memory store ready", "backend", emb.Backend, "model", emb.Model)
		return store
	}

	return agent.NewMarkdownFileStore(dir)
}

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

// toolCapabilities returns the names of all tools currently registered in r.
// This is used to populate the capabilities field of the hub registration message.
func toolCapabilities(r *tools.Registry) []string {
	defs := r.Definitions()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

func buildContextOptimizer(cfg config.AgentConfig, llmClient llm.Client, logger *slog.Logger) *agent.ContextOptimizer {
	var summarizer agent.Summarizer
	if cfg.LLM.ContextSummarizeThreshold > 0 {
		summarizerClient := llmClient
		if cfg.LLM.ContextSummarizeModel != "" {
			summarizeCfg := cfg.LLM
			summarizeCfg.Model = cfg.LLM.ContextSummarizeModel
			sc, err := llm.NewClientFromConfig(summarizeCfg, "", logger)
			if err != nil {
				logger.Warn("failed to create summarizer LLM client, skipping", "error", err)
			} else {
				summarizerClient = sc
				logger.Info("summarizer using separate model", "model", cfg.LLM.ContextSummarizeModel)
			}
		}
		summarizer = agent.NewLLMSummarizer(summarizerClient)
	}
	var pruneLevels []agent.PruneLevel
	for _, s := range cfg.LLM.ContextPruneLevels {
		pruneLevels = append(pruneLevels, agent.PruneLevel(s))
	}
	return agent.NewContextOptimizer(agent.ContextOptimizerConfig{
		TokenBudget:        cfg.LLM.ContextTokenBudget,
		SummarizeThreshold: cfg.LLM.ContextSummarizeThreshold,
		SummarizeTurns:     cfg.LLM.ContextSummarizeTurns,
		PruneLevels:        pruneLevels,
	}, summarizer, logger)
}
