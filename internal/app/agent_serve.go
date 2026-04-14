package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	clidaemon "github.com/zsomething/zlaw/internal/adapters/daemon"
	"github.com/zsomething/zlaw/internal/adapters/telegram"
	"github.com/zsomething/zlaw/internal/agent"
	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/cron"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/messaging"
	"github.com/zsomething/zlaw/internal/push"
	"github.com/zsomething/zlaw/internal/session"
	"github.com/zsomething/zlaw/internal/tools/builtin"
	"github.com/zsomething/zlaw/internal/transport"
	"github.com/zsomething/zlaw/internal/version"
)

// ServeAgent wires up an agent from agentDir and runs it as a daemon
// (Unix socket + optional Telegram).
func ServeAgent(ctx context.Context, agentDir string, logger *slog.Logger) error {
	logger.Info("agent dir resolved", "path", agentDir)

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

	llmClient, err := llm.NewClientFromConfig(cfg.LLM, "", logger)
	if err != nil {
		return fmt.Errorf("create llm client: %w", err)
	}
	logger.Info("llm configured", "backend", cfg.LLM.Backend, "model", cfg.LLM.Model, "auth_profile", cfg.LLM.AuthProfile)

	// cronWriter is created early so cron tools are registered before the
	// allowlist is applied. The scheduler field is wired after construction.
	cronWriter := &cronWriterImpl{agentDir: agentDir}

	registry, delegateTool := buildToolRegistry(ctx, cfg, loader, logger)
	registry.Register(builtin.ListCronjobs{Reader: cronWriter})
	registry.Register(builtin.CreateCronjob{Writer: cronWriter})
	registry.Register(builtin.DeleteCronjob{Writer: cronWriter})

	// Discovery tools — read-only, available to all agents. Messenger injected below.
	listAgentsTool := &builtin.ListAgents{Registry: builtin.NewAgentRegistry(nil)}
	getAgentTool := &builtin.GetAgent{Registry: builtin.NewAgentRegistry(nil)}
	registry.Register(listAgentsTool)
	registry.Register(getAgentTool)

	applyToolConfig(cfg, registry, logger)

	history, err := buildHistory(cfg.Agent.ID, "daemon", logger)
	if err != nil {
		return fmt.Errorf("create session history: %w", err)
	}

	ag := buildAgent(ctx, cfg, llmClient, registry, history, stickyBlocks, nil, logger)

	sysPromptFn := func() string { return *promptPtr.Load() }

	sessionManager := session.NewManager(
		agentRunner{ag},
		sysPromptFn,
		logger,
	)

	runDir := filepath.Join(config.ZlawHome(), "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	// ZLAW_AGENT is injected by the hub and matches the ACL username exactly.
	// Prefer it so NATS auth works regardless of what agent.toml says.
	name := os.Getenv("ZLAW_AGENT")
	if name == "" {
		name = cfg.Agent.ID
	}
	if name == "" {
		name = "default"
	}
	sockPath := filepath.Join(runDir, name+".sock")
	pidPath := filepath.Join(runDir, name+".pid")
	t := transport.NewUnixTransport(sockPath)
	d := clidaemon.New(t, sessionManager, pidPath, logger)

	pushRegistry := push.NewRegistry()

	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		tgAdapter := telegram.NewAdapter(token, sessionManager, logger)
		tgAdapter.SetHistoryManager(history)
		pushRegistry.Register("telegram", tgAdapter)
		go func() {
			if err := tgAdapter.Run(ctx); err != nil {
				logger.Error("telegram adapter stopped", "error", err)
			}
		}()
	}

	scheduler := cron.NewScheduler(
		cronRunner{ag: ag, sysPromptFn: sysPromptFn},
		pushRegistry,
		sysPromptFn,
		logger,
	)

	initialCron, err := config.LoadCronConfig(agentDir)
	if err != nil {
		logger.Warn("cron: failed to load initial cron.toml, starting with no jobs", "err", err)
	} else {
		scheduler.Reload(initialCron.Jobs)
	}
	loader.SetCronChangeHandler(func(c config.CronConfig) {
		scheduler.Reload(c.Jobs)
	})

	cronWriter.scheduler = scheduler
	go scheduler.Run(ctx)
	logger.Info("cron scheduler started")

	go func() {
		if err := loader.Watch(ctx); err != nil {
			logger.Error("config watcher stopped", "error", err)
		}
	}()

	// Hub-connected mode: connect to NATS, publish registration, handle inbox.
	if natsURL := os.Getenv("ZLAW_NATS_URL"); natsURL != "" {
		natsToken := os.Getenv("ZLAW_NATS_CREDS")
		nm, err := messaging.NewNATSMessenger(natsURL, name, natsToken)
		if err != nil {
			return fmt.Errorf("connect to hub NATS: %w", err)
		}
		defer nm.Close()

		// Wire messenger into delegate tool so it can send tasks to other agents.
		regCache := agent.NewRegistryCache(logger)
		delegateTool.Messenger = nm
		delegateTool.Registry = regCache

		// Inject messenger into discovery tools so they can query zlaw.registry.list.
		listAgentsTool.Registry = builtin.NewAgentRegistry(nm)
		getAgentTool.Registry = builtin.NewAgentRegistry(nm)

		go func() {
			if err := regCache.Start(ctx, nm); err != nil && ctx.Err() == nil {
				logger.Warn("registry cache stopped unexpectedly", "err", err)
			}
		}()

		caps := toolCapabilities(registry)
		hubClient := agent.NewHubClient(
			name,
			version.Version,
			caps,
			cfg.Agent.Roles,
			nm,
			cronRunner{ag: ag, sysPromptFn: sysPromptFn},
			sysPromptFn,
			logger,
		)
		go func() {
			if err := hubClient.Start(ctx); err != nil {
				logger.Error("hub client stopped", "err", err)
			}
		}()
		logger.Info("hub client started", "nats_url", natsURL)
	}

	logger.Info("daemon starting", "agent", name, "socket", sockPath)

	drainTimeout := time.Duration(cfg.Serve.ShutdownTimeoutSec) * time.Second
	return d.Serve(ctx, drainTimeout)
}

// ── Internal adapters ─────────────────────────────────────────────────────────

// agentRunner adapts *agent.Agent to session.AgentRunner.
type agentRunner struct{ ag *agent.Agent }

func (w agentRunner) RunStream(
	ctx context.Context,
	sessionID, input, systemPrompt string,
	handler llm.StreamHandler,
) (string, error) {
	result, err := w.ag.RunStream(ctx, sessionID, input, systemPrompt, handler)
	return result.Text, err
}

var _ session.AgentRunner = agentRunner{}

// cronRunner adapts *agent.Agent to cron.AgentRunner.
type cronRunner struct {
	ag          *agent.Agent
	sysPromptFn func() string
}

func (r cronRunner) Run(ctx context.Context, sessionID, input, _ string) (string, error) {
	result, err := r.ag.Run(ctx, sessionID, input, r.sysPromptFn())
	return result.Text, err
}

var _ cron.AgentRunner = cronRunner{}

// cronWriterImpl satisfies builtin.CronWriter.
type cronWriterImpl struct {
	agentDir  string
	scheduler *cron.Scheduler
}

func (c *cronWriterImpl) AgentDir() string { return c.agentDir }
func (c *cronWriterImpl) ReloadCron() {
	cfg, err := config.LoadCronConfig(c.agentDir)
	if err != nil {
		return
	}
	c.scheduler.Reload(cfg.Jobs)
}

var _ builtin.CronWriter = (*cronWriterImpl)(nil)
