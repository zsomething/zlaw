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
	"github.com/zsomething/zlaw/internal/credentials"
	"github.com/zsomething/zlaw/internal/cron"
	"github.com/zsomething/zlaw/internal/llm"
	"github.com/zsomething/zlaw/internal/messaging"
	"github.com/zsomething/zlaw/internal/push"
	"github.com/zsomething/zlaw/internal/session"
	"github.com/zsomething/zlaw/internal/slashcmd"
	"github.com/zsomething/zlaw/internal/tools/builtin"
	"github.com/zsomething/zlaw/internal/transport"
	"github.com/zsomething/zlaw/internal/version"
)

// ServeAgent wires up an agent from agentDir and runs it as a daemon.
// workspaceDir contains SOUL.md and IDENTITY.md; agent has read access.
// agentDir contains agent.toml and credentials.toml; agent accesses via env var only.
func ServeAgent(ctx context.Context, agentDir string, workspaceDir string, logger *slog.Logger) error {
	logger.Info("agent dir resolved", "path", agentDir)
	logger.Info("workspace resolved", "path", workspaceDir)

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
	loader, err := config.NewLoader(agentDir, workspaceDir, onChange, logger)
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

	// Agent discovery tools (list_agents, get_agent) are registered by buildToolRegistry.

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

	// Register adapters from config (multi-adapter support).
	credPath := credentials.DefaultCredentialsPath()
	for _, adapterCfg := range adaptersFromConfig(cfg) {
		registerAdapterFromConfig(ctx, adapterCfg, sessionManager, history, pushRegistry, credPath, logger)
	}

	// Legacy: also check TELEGRAM_BOT_TOKEN for backward compat.
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

		// Inject messenger into discovery tools that were registered by buildToolRegistry.
		if listAgents := registry.Get("list_agents"); listAgents != nil {
			if la, ok := listAgents.(*builtin.ListAgents); ok {
				la.Registry = builtin.NewAgentRegistry(nm)
			}
		}
		if getAgent := registry.Get("get_agent"); getAgent != nil {
			if ga, ok := getAgent.(*builtin.GetAgent); ok {
				ga.Registry = builtin.NewAgentRegistry(nm)
			}
		}

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

// adaptersFromConfig returns the list of adapter configs.
// When Adapters is non-empty, it returns those (multi-adapter mode).
// Otherwise, it returns a single entry based on the legacy Type field.
func adaptersFromConfig(cfg config.AgentConfig) []config.AdapterInstanceConfig {
	return cfg.Adapter
}

// registerAdapterFromConfig loads adapter credentials and registers the adapter.
func registerAdapterFromConfig(
	ctx context.Context,
	adapterCfg config.AdapterInstanceConfig,
	sessionManager *session.Manager,
	history slashcmd.HistoryManager,
	pushRegistry *push.Registry,
	credPath string,
	logger *slog.Logger,
) {
	var token string

	if adapterCfg.AuthProfile != "" {
		profile, err := credentials.GetProfile(credPath, adapterCfg.AuthProfile)
		if err != nil {
			logger.Warn("adapter credential profile not found, skipping",
				"adapter_type", adapterCfg.Type,
				"auth_profile", adapterCfg.AuthProfile,
				"err", err,
			)
			return
		}

		// Adapter-specific key lookup.
		switch adapterCfg.Type {
		case "telegram":
			token = profile.GetData("telegram_bot_token")
		case "fizzy":
			token = profile.GetData("fizzy_api_key")
		// Add other adapter types here as needed.
		default:
			logger.Debug("unknown adapter type for credential loading", "type", adapterCfg.Type)
		}

		if token == "" {
			logger.Warn("adapter profile has no required credential",
				"adapter_type", adapterCfg.Type,
				"auth_profile", adapterCfg.AuthProfile,
			)
			return
		}
	}

	// Register the adapter.
	switch adapterCfg.Type {
	case "telegram":
		tgAdapter := telegram.NewAdapter(token, sessionManager, logger)
		tgAdapter.SetHistoryManager(history)
		pushRegistry.Register("telegram", tgAdapter)
		go func() {
			if err := tgAdapter.Run(ctx); err != nil {
				logger.Error("telegram adapter stopped", "error", err)
			}
		}()
		logger.Info("telegram adapter registered", "auth_profile", adapterCfg.AuthProfile)
	// Add other adapter types here.
	default:
		logger.Debug("adapter type not yet implemented", "type", adapterCfg.Type)
	}
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
