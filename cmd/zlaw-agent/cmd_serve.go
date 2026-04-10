package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	clidaemon "github.com/chickenzord/zlaw/internal/adapters/daemon"
	"github.com/chickenzord/zlaw/internal/adapters/telegram"
	"github.com/chickenzord/zlaw/internal/agent"
	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/llm"
	"github.com/chickenzord/zlaw/internal/session"
	"github.com/chickenzord/zlaw/internal/tools"
	"github.com/chickenzord/zlaw/internal/tools/builtin"
	"github.com/chickenzord/zlaw/internal/transport"
)

// agentWrapper adapts *agent.Agent to session.AgentRunner.
// It extracts only the response text from agent.Result, keeping the session
// package free of a dependency on internal/agent.
type agentWrapper struct{ ag *agent.Agent }

func (w agentWrapper) RunStream(
	ctx context.Context,
	sessionID, input, systemPrompt string,
	handler llm.StreamHandler,
) (string, error) {
	result, err := w.ag.RunStream(ctx, sessionID, input, systemPrompt, handler)
	return result.Text, err
}

// compile-time check: agentWrapper satisfies session.AgentRunner.
var _ session.AgentRunner = agentWrapper{}

func runServe(ctx context.Context, args []string, agentName, agentDir string, logger *slog.Logger) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	resolvedDir, err := resolveAgentDir(agentName, agentDir)
	if err != nil {
		return err
	}
	agentDir = resolvedDir
	logger.Info("agent dir resolved", "path", agentDir)

	// --- Load config (same pattern as runRun) ---
	var promptPtr atomic.Pointer[string]
	var stickyBlocks []agent.StickyBlock

	onChange := func(_ config.AgentConfig, p config.Personality) {
		s := agent.BuildSystemPrompt(nil, p)
		promptPtr.Store(&s)
		logger.Info("system prompt reloaded",
			"soul_len", len(p.Soul),
			"identity_len", len(p.Identity),
			"prompt_len", len(s),
		)
	}
	loader, err := config.NewLoader(agentDir, onChange, logger)
	if err != nil {
		return fmt.Errorf("create config loader: %w", err)
	}
	cfg, personality, err := loader.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger.Info("personality loaded",
		"soul_len", len(personality.Soul),
		"identity_len", len(personality.Identity),
	)
	logger.Debug("personality content",
		"soul", personality.Soul,
		"identity", personality.Identity,
	)

	if cfg.Sticky.ProactiveMemorySave {
		stickyBlocks = append(stickyBlocks, agent.StickyBlock{
			Name:    "memory-behavior",
			Content: agent.StickyProactiveMemorySave,
		})
		logger.Info("sticky block enabled", "name", "memory-behavior")
	}

	initial := agent.BuildSystemPrompt(nil, personality)
	promptPtr.Store(&initial)
	logger.Debug("initial system prompt", "prompt", initial)

	// --- Build LLM client ---
	llmClient, err := llm.NewClientFromConfig(cfg.LLM, "", logger)
	if err != nil {
		return fmt.Errorf("create llm client: %w", err)
	}
	logger.Info("llm configured", "backend", cfg.LLM.Backend, "model", cfg.LLM.Model, "auth_profile", cfg.LLM.AuthProfile)

	// --- Build tool registry (same set as runRun) ---
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
	memStore := buildMemoryStore(cfg.Agent.Name, logger)
	if memStore != nil {
		registry.Register(builtin.MemorySave{Store: memStore})
		registry.Register(builtin.MemoryRecall{Store: memStore})
		registry.Register(builtin.MemoryDelete{Store: memStore})
	}
	if len(cfg.Tools.Allowed) > 0 {
		registry.SetAllowlist(cfg.Tools.Allowed)
		logger.Info("tool allowlist enforced", "allowed", cfg.Tools.Allowed)
	}
	registry.SetMaxResultBytes(cfg.Tools.MaxResultBytes)

	// --- Build agent ---
	// The daemon uses a single History shared across all sessions (different sessionIDs).
	history, err := buildHistory(cfg.Agent.Name, "daemon", logger)
	if err != nil {
		return fmt.Errorf("create session history: %w", err)
	}
	ag := agent.New(cfg.Agent.Name, llmClient, registry, history, logger)
	ag.SetStickyBlocks(stickyBlocks)
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

	// --- Build session manager ---
	sessionManager := session.NewManager(
		agentWrapper{ag},
		func() string { return *promptPtr.Load() },
		logger,
	)

	// --- Build Unix transport ---
	runDir := filepath.Join(config.ZlawHome(), "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	name := cfg.Agent.Name
	if name == "" {
		name = "default"
	}
	sockPath := filepath.Join(runDir, name+".sock")
	pidPath := filepath.Join(runDir, name+".pid")
	t := transport.NewUnixTransport(sockPath)

	// --- Build daemon ---
	d := clidaemon.New(t, sessionManager, pidPath, logger)

	// --- Start Telegram adapter (if configured) ---
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		tgAdapter := telegram.NewAdapter(token, sessionManager, logger)
		go func() {
			if err := tgAdapter.Run(ctx); err != nil {
				logger.Error("telegram adapter stopped", "error", err)
			}
		}()
	}

	// --- Start config hot-reload ---
	go func() {
		if err := loader.Watch(ctx); err != nil {
			logger.Error("config watcher stopped", "error", err)
		}
	}()

	logger.Info("daemon starting", "agent", name, "socket", sockPath)

	// Derive drain timeout from config (default 60 s).
	drainTimeout := time.Duration(cfg.Serve.ShutdownTimeoutSec) * time.Second

	// Serve blocks until ctx is cancelled (SIGTERM / SIGINT), then drains.
	return d.Serve(ctx, drainTimeout)
}
