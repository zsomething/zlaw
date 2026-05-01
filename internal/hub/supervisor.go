package hub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/credentials"
	"github.com/zsomething/zlaw/internal/messaging"
)

const (
	// backoffBase is the initial restart delay.
	backoffBase = time.Second
	// backoffMax is the maximum restart delay.
	backoffMax = 5 * time.Minute
)

// AgentStatus describes the current state of a supervised agent.
type AgentStatus struct {
	ID      string
	Running bool
	PID     int
	LastErr error
}

// Supervisor manages agent process lifecycles on behalf of the hub.
// It spawns, monitors, and optionally restarts agent processes according
// to per-agent restart policies.
type Supervisor struct {
	cfg             config.HubConfig
	natsURL         string
	selfBin         string // path to the hub's own executable
	credentialsPath string // path to credentials.toml; "" → default
	agentTokens     AgentTokens
	logger          *slog.Logger
	messenger       messaging.Messenger // for publishing agent logs to NATS
	noColor         bool
	registry        *Registry // for runtime auth profile lookup

	mu     sync.Mutex
	agents map[string]*managedAgent
}

// managedAgent holds the runtime state of a single supervised agent.
type managedAgent struct {
	entry config.AgentEntry

	mu      sync.Mutex
	cmd     *exec.Cmd
	running bool
	pid     int
	lastErr error

	// stopOnce ensures the stop channel is closed exactly once.
	stopOnce sync.Once
	// stopCh is closed to signal the monitor goroutine to stop respawning.
	stopCh chan struct{}
}

func (m *managedAgent) setRunning(cmd *exec.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cmd = cmd
	m.running = true
	if cmd != nil && cmd.Process != nil {
		m.pid = cmd.Process.Pid
	}
	m.lastErr = nil
}

func (m *managedAgent) setStopped(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.pid = 0
	m.lastErr = err
}

func (m *managedAgent) status() AgentStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return AgentStatus{
		ID:      m.entry.ID,
		Running: m.running,
		PID:     m.pid,
		LastErr: m.lastErr,
	}
}

// NewSupervisor creates a Supervisor for the given hub configuration.
// natsURL is the NATS client URL that will be injected into each agent process
// as ZLAW_NATS_URL. selfBin is the path to the hub executable used to spawn
// agents when no custom binary is set on the AgentEntry.
// credentialsPath is the path to credentials.toml used for credential injection;
// pass "" to use the default path from the auth package.
// agentTokens maps agent name to its NATS token; pass nil to skip token injection.
// registry is used for runtime auth profile lookup from agent registration; pass nil to skip.
func NewSupervisor(cfg config.HubConfig, natsURL, selfBin, credentialsPath string, agentTokens AgentTokens, logger *slog.Logger, registry *Registry) *Supervisor {
	if agentTokens == nil {
		agentTokens = make(AgentTokens)
	}
	return &Supervisor{
		cfg:             cfg,
		natsURL:         natsURL,
		selfBin:         selfBin,
		credentialsPath: credentialsPath,
		agentTokens:     agentTokens,
		logger:          logger,
		noColor:         DefaultNoColor(),
		registry:        registry,
		agents:          make(map[string]*managedAgent),
	}
}

// NewSupervisorWithOptions creates a Supervisor with explicit options.
func NewSupervisorWithOptions(cfg config.HubConfig, natsURL, selfBin, credentialsPath string, agentTokens AgentTokens, logger *slog.Logger, noColor bool, registry *Registry) *Supervisor {
	if agentTokens == nil {
		agentTokens = make(AgentTokens)
	}
	return &Supervisor{
		cfg:             cfg,
		natsURL:         natsURL,
		selfBin:         selfBin,
		credentialsPath: credentialsPath,
		agentTokens:     agentTokens,
		logger:          logger,
		noColor:         noColor,
		registry:        registry,
		agents:          make(map[string]*managedAgent),
	}
}

// NewSupervisorWithMessenger creates a Supervisor with a messenger for log publishing.
func NewSupervisorWithMessenger(cfg config.HubConfig, natsURL, selfBin, credentialsPath string, agentTokens AgentTokens, logger *slog.Logger, noColor bool, messenger messaging.Messenger, registry *Registry) *Supervisor {
	if agentTokens == nil {
		agentTokens = make(AgentTokens)
	}
	return &Supervisor{
		cfg:             cfg,
		natsURL:         natsURL,
		selfBin:         selfBin,
		credentialsPath: credentialsPath,
		agentTokens:     agentTokens,
		logger:          logger,
		messenger:       messenger,
		noColor:         noColor,
		registry:        registry,
		agents:          make(map[string]*managedAgent),
	}
}

// Start spawns all configured agents and begins monitoring them.
// It returns once all agents have been launched (or failed to launch the first
// time). Monitoring continues in background goroutines until ctx is cancelled.
func (s *Supervisor) Start(ctx context.Context) error {
	for i := range s.cfg.Agents {
		entry := s.cfg.Agents[i]
		ma := &managedAgent{
			entry:  entry,
			stopCh: make(chan struct{}),
		}
		s.mu.Lock()
		s.agents[entry.ID] = ma
		s.mu.Unlock()

		go s.monitor(ctx, ma)
	}
	return nil
}

// Spawn adds a new agent entry and starts monitoring it. It returns an error if
// an agent with the same name is already supervised.
func (s *Supervisor) Spawn(ctx context.Context, entry config.AgentEntry) error {
	s.mu.Lock()
	if _, exists := s.agents[entry.ID]; exists {
		s.mu.Unlock()
		return fmt.Errorf("supervisor: agent %q is already supervised", entry.ID)
	}
	ma := &managedAgent{
		entry:  entry,
		stopCh: make(chan struct{}),
	}
	s.agents[entry.ID] = ma
	s.mu.Unlock()

	go s.monitor(ctx, ma)
	return nil
}

// Stop signals the agent to stop and does not restart it.
func (s *Supervisor) Stop(agentID string) error {
	ma := s.get(agentID)
	if ma == nil {
		return fmt.Errorf("supervisor: agent %q not found", agentID)
	}
	ma.stopOnce.Do(func() { close(ma.stopCh) })
	return s.kill(ma)
}

// Restart stops the named agent and spawns a fresh process.
func (s *Supervisor) Restart(name string) error {
	ma := s.get(name)
	if ma == nil {
		return fmt.Errorf("supervisor: agent %q not found", name)
	}
	if err := s.kill(ma); err != nil {
		return err
	}
	// Reset stopCh so the monitor goroutine picks up a fresh one.
	ma.mu.Lock()
	ma.stopOnce = sync.Once{}
	ma.stopCh = make(chan struct{})
	ma.mu.Unlock()
	return nil
}

// Remove permanently removes the named agent from supervision. It stops the
// agent first if running, then removes it from the internal map so it will not
// be restarted. Use this when permanently deleting an agent.
func (s *Supervisor) Remove(name string) error {
	ma := s.get(name)
	if ma == nil {
		return fmt.Errorf("supervisor: agent %q not found", name)
	}
	// Signal the monitor goroutine to stop and remove from map.
	ma.stopOnce.Do(func() { close(ma.stopCh) })
	_ = s.kill(ma)
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, name)
	return nil
}

// Status returns the current state of the named agent.
func (s *Supervisor) Status(agentID string) (AgentStatus, error) {
	ma := s.get(agentID)
	if ma == nil {
		return AgentStatus{}, fmt.Errorf("supervisor: agent %q not found", agentID)
	}
	return ma.status(), nil
}

// Statuses returns the status of every supervised agent.
func (s *Supervisor) Statuses() []AgentStatus {
	s.mu.Lock()
	names := make([]string, 0, len(s.agents))
	for n := range s.agents {
		names = append(names, n)
	}
	s.mu.Unlock()

	out := make([]AgentStatus, 0, len(names))
	for _, id := range names {
		if st, err := s.Status(id); err == nil {
			out = append(out, st)
		}
	}
	return out
}

// get retrieves the managedAgent by name (nil if not found).
func (s *Supervisor) get(name string) *managedAgent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agents[name]
}

// kill sends SIGTERM to the agent process (if running).
func (s *Supervisor) kill(ma *managedAgent) error {
	ma.mu.Lock()
	cmd := ma.cmd
	running := ma.running
	ma.mu.Unlock()

	if !running || cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		// Process may have already exited.
		if !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("signal agent %s: %w", ma.entry.ID, err)
		}
	}
	return nil
}

// monitor runs in a goroutine and manages the agent's process lifecycle:
// spawn → wait → restart (according to policy and backoff).
func (s *Supervisor) monitor(ctx context.Context, ma *managedAgent) {
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ma.stopCh:
			return
		default:
		}

		if attempt > 0 {
			delay := BackoffDelay(attempt)
			s.logger.Info("supervisor: waiting before restart",
				"agent", ma.entry.ID,
				"attempt", attempt,
				"delay", delay,
			)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			case <-ma.stopCh:
				return
			}
		}

		cmd, err := s.buildCmd(ma.entry)
		if err != nil {
			s.logger.Error("supervisor: build command failed",
				"agent", ma.entry.ID, "err", err)
			ma.setStopped(err)
			return
		}

		if err := cmd.Start(); err != nil {
			s.logger.Error("supervisor: start failed",
				"agent", ma.entry.ID, "err", err)
			ma.setStopped(err)
			attempt++
			if !s.shouldRestart(ma.entry, err) {
				return
			}
			continue
		}

		ma.setRunning(cmd)
		s.logger.Info("supervisor: agent started",
			"agent", ma.entry.ID,
			"pid", cmd.Process.Pid,
			"attempt", attempt,
		)

		exitErr := cmd.Wait()
		ma.setStopped(exitErr)

		if exitErr != nil {
			s.logger.Warn("supervisor: agent exited with error",
				"agent", ma.entry.ID,
				"err", exitErr,
			)
		} else {
			s.logger.Info("supervisor: agent exited cleanly",
				"agent", ma.entry.ID,
			)
		}

		if !s.shouldRestart(ma.entry, exitErr) {
			return
		}
		attempt++
	}
}

// buildCmd constructs the exec.Cmd for the given agent entry.
func (s *Supervisor) buildCmd(entry config.AgentEntry) (*exec.Cmd, error) {
	bin := entry.Binary
	if bin == "" {
		bin = s.selfBin
	}
	if bin == "" {
		return nil, fmt.Errorf("no binary configured for agent %q and selfBin is empty", entry.ID)
	}

	agentDir := resolveAgentDir(entry)

	// AgentEntry.Dir must always be absolute (written by ctl at create time).
	if !filepath.IsAbs(agentDir) {
		return nil, fmt.Errorf("agent %q dir must be absolute, got %q", entry.ID, agentDir)
	}

	var args []string
	if entry.Binary == "" {
		// Using hub binary: run as sub-command "agent serve"
		args = []string{"agent", "serve", "--agent", entry.ID}
	} else {
		// Custom binary: pass agent name via env, no sub-command assumed.
		args = nil
	}

	cmd := exec.Command(bin, args...) //nolint:gosec

	// Build environment: inherit everything, override/add hub-specific vars.
	// ZLAW_LOG_FORMAT=json makes the agent output structured JSON that we relay
	// with PrettyHandler. This allows unified log formatting at the hub.
	env := os.Environ()
	env = SetEnv(env, "ZLAW_AGENT", entry.ID)
	env = SetEnv(env, "ZLAW_NATS_URL", s.natsURL)
	env = SetEnv(env, "ZLAW_LOG_FORMAT", "json")
	env = SetEnv(env, "ZLAW_NO_COLOR", "1") // colors applied by hub's PrettyHandler
	env = SetEnv(env, "ZLAW_AGENT_HOME", agentDir)

	// Pipe agent stdout/stderr through JSON log reader for unified pretty output.
	// If messenger is set, logs are also published to NATS for 'zlaw agent logs' clients.
	label := fmt.Sprintf("[agent:%s]", entry.ID)
	color := AgentColor(entry.ID)
	stdoutWriter := newAgentLogWriter(label, color, s.noColor)
	stderrWriter := newAgentLogWriter(label, color, s.noColor)
	if s.messenger != nil {
		stdoutWriter = stdoutWriter.withMessenger(s.messenger, entry.ID)
		stderrWriter = stderrWriter.withMessenger(s.messenger, entry.ID)
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// Inject credentials from the global credentials.toml.
	// Hub reads from $ZLAW_HOME/credentials.toml and writes filtered profiles
	// to $ZLAW_HOME/run/credentials/<id>.toml for injection.
	// Auth profiles are sourced from AgentEntry (zlaw.toml) with registry fallback.
	profiles := entry.AuthProfiles
	if len(profiles) == 0 && s.registry != nil {
		if regEntry, ok := s.registry.Get(entry.ID); ok {
			profiles = regEntry.AuthProfiles
		}
	}

	if len(profiles) > 0 {
		runtimeCredsPath := filepath.Join(config.ZlawHome(), "run", "credentials", entry.ID+".toml")
		if err := injectCredentialsFromGlobal(profiles, runtimeCredsPath); err != nil {
			return nil, fmt.Errorf("credential injection for agent %q: %w", entry.ID, err)
		}
		env = SetEnv(env, "ZLAW_CREDENTIALS_FILE", runtimeCredsPath)
	}

	// Inject the NATS token for this agent so NATSMessenger can authenticate.
	if token, ok := s.agentTokens[entry.ID]; ok && token != "" {
		env = SetEnv(env, "ZLAW_NATS_CREDS", token)
	}

	cmd.Env = env

	return cmd, nil
}

// shouldRestart decides whether to restart the agent given the exit error.
func (s *Supervisor) shouldRestart(entry config.AgentEntry, exitErr error) bool {
	policy := entry.RestartPolicy
	if policy == "" {
		policy = config.RestartOnFailure
	}
	switch policy {
	case config.RestartAlways:
		return true
	case config.RestartOnFailure:
		return exitErr != nil
	case config.RestartNever:
		return false
	default:
		return false
	}
}

// BackoffDelay returns the delay before the nth restart attempt.
// Uses exponential backoff capped at backoffMax.
func BackoffDelay(attempt int) time.Duration {
	exp := math.Pow(2, float64(attempt-1))
	if math.IsInf(exp, 1) || exp > float64(backoffMax) {
		return backoffMax
	}
	d := time.Duration(float64(backoffBase) * exp)
	if d <= 0 || d > backoffMax {
		return backoffMax
	}
	return d
}

// injectCredentialsFromGlobal reads the named auth profiles from the global
// credentials.toml and writes a filtered copy to runtimeCredsPath.
func injectCredentialsFromGlobal(profiles []string, runtimeCredsPath string) error {
	globalCredsPath := filepath.Join(config.ZlawHome(), "credentials.toml")
	store, err := credentials.LoadStore(globalCredsPath)
	if err != nil {
		return fmt.Errorf("load global credentials: %w", err)
	}

	// Filter to only needed profiles.
	filtered := credentials.CredentialStore{
		Profiles: make(map[string]credentials.CredentialProfile, len(profiles)),
	}
	for _, name := range profiles {
		profile, ok := store.Profiles[name]
		if !ok {
			return fmt.Errorf("auth profile %q not found in global credentials", name)
		}
		filtered.Profiles[name] = profile
	}

	// Write filtered credentials to runtime dir.
	runDir := filepath.Join(config.ZlawHome(), "run", "credentials")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create runtime credentials dir: %w", err)
	}

	return credentials.SaveStore(runtimeCredsPath, filtered)
}

// resolveAgentDir returns the agent directory from entry.Dir.
// entry.Dir must be absolute — caller is responsible for validation.
func resolveAgentDir(entry config.AgentEntry) string {
	return entry.Dir
}

// SetEnv sets or replaces key=value in an environment slice.
func SetEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
