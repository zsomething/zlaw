package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/zsomething/zlaw/internal/secrets"
)

const (
	backoffBase = time.Second
	backoffMax  = 5 * time.Minute
)

// SubprocessExecutor spawns agents as child processes with self-monitoring.
type SubprocessExecutor struct {
	agents map[string]*subprocessAgent
	mu     sync.Mutex
	logger *slog.Logger
}

// subprocessAgent holds runtime state for a single subprocess agent.
type subprocessAgent struct {
	cfg      AgentConfig
	cmd      *exec.Cmd
	mu       sync.Mutex
	running  bool
	pid      int
	lastErr  error
	stopCh   chan struct{}
	stopOnce sync.Once
}

func (s *SubprocessExecutor) get(id string) *subprocessAgent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agents[id]
}

func (s *SubprocessExecutor) set(id string, ag *subprocessAgent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents[id] = ag
}

// Start launches an agent as a subprocess.
func (s *SubprocessExecutor) Start(ctx context.Context, cfg AgentConfig) error {
	ag := &subprocessAgent{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	s.set(cfg.ID, ag)

	go s.monitor(ctx, ag)
	return nil
}

// Stop terminates the agent process.
func (s *SubprocessExecutor) Stop(ctx context.Context, id string) error {
	ag := s.get(id)
	if ag == nil {
		return fmt.Errorf("agent %q not found", id)
	}

	ag.stopOnce.Do(func() { close(ag.stopCh) })

	ag.mu.Lock()
	cmd := ag.cmd
	ag.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal agent %s: %w", id, err)
	}
	return nil
}

// Status returns the current state of the agent.
func (s *SubprocessExecutor) Status(ctx context.Context, id string) (Status, error) {
	ag := s.get(id)
	if ag == nil {
		return Status{}, fmt.Errorf("agent %q not found", id)
	}

	ag.mu.Lock()
	defer ag.mu.Unlock()
	return Status{
		ID:      id,
		Running: ag.running,
		PID:     ag.pid,
		Error:   ag.lastErr,
	}, nil
}

// Logs returns stdout/stderr as a combined stream.
func (s *SubprocessExecutor) Logs(ctx context.Context, id string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

// monitor manages the agent process lifecycle.
func (s *SubprocessExecutor) monitor(ctx context.Context, ag *subprocessAgent) {
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ag.stopCh:
			return
		default:
		}

		if attempt > 0 {
			delay := backoffDelay(attempt)
			s.logger.Info("agent waiting before restart",
				"agent", ag.cfg.ID,
				"attempt", attempt,
				"delay", delay,
			)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			case <-ag.stopCh:
				return
			}
		}

		cmd, err := s.buildCmd(ag.cfg)
		if err != nil {
			s.logger.Error("agent build command failed",
				"agent", ag.cfg.ID, "err", err)
			return
		}

		if err := cmd.Start(); err != nil {
			s.logger.Error("agent start failed",
				"agent", ag.cfg.ID, "err", err)
			attempt++
			if !s.shouldRestart(ag.cfg.RestartPolicy, err) {
				return
			}
			continue
		}

		ag.mu.Lock()
		ag.cmd = cmd
		ag.running = true
		if cmd.Process != nil {
			ag.pid = cmd.Process.Pid
		}
		ag.mu.Unlock()

		s.logger.Info("agent started",
			"agent", ag.cfg.ID,
			"pid", cmd.Process.Pid,
			"attempt", attempt,
		)

		exitErr := cmd.Wait()
		ag.mu.Lock()
		ag.running = false
		ag.pid = 0
		ag.lastErr = exitErr
		ag.mu.Unlock()

		if exitErr != nil {
			s.logger.Warn("agent exited with error",
				"agent", ag.cfg.ID,
				"err", exitErr,
			)
		}

		if !s.shouldRestart(ag.cfg.RestartPolicy, exitErr) {
			return
		}
		attempt++
	}
}

func (s *SubprocessExecutor) buildCmd(cfg AgentConfig) (*exec.Cmd, error) {
	bin := cfg.Binary
	if bin == "" {
		bin = os.Args[0]
	}
	if bin == "" {
		return nil, fmt.Errorf("no binary configured for agent %q", cfg.ID)
	}

	agentDir := cfg.Dir
	if !filepath.IsAbs(agentDir) {
		return nil, fmt.Errorf("agent %q dir must be absolute", cfg.ID)
	}

	args := []string{"agent", "serve", "--agent", cfg.ID}
	cmd := exec.Command(bin, args...) //nolint:gosec

	// Build environment.
	env := os.Environ()
	env = setEnv(env, "ZLAW_AGENT", cfg.ID)
	env = setEnv(env, "ZLAW_NATS_URL", cfg.NATSURL)
	env = setEnv(env, "ZLAW_LOG_FORMAT", "json")
	env = setEnv(env, "ZLAW_AGENT_HOME", agentDir)

	// Load secrets and inject.
	sec, err := secrets.Load(secrets.DefaultSecretsPath())
	if err != nil {
		return nil, fmt.Errorf("load secrets: %w", err)
	}
	for _, ev := range cfg.EnvVars {
		value := sec.Get(ev.FromSecret)
		if value == "" {
			return nil, fmt.Errorf("secret %q not found", ev.FromSecret)
		}
		env = setEnv(env, ev.Name, value)
	}

	// Inject NATS token if provided.
	if cfg.NATSToken != "" {
		env = setEnv(env, "ZLAW_NATS_CREDS", cfg.NATSToken)
	}

	cmd.Env = env

	return cmd, nil
}

func (s *SubprocessExecutor) shouldRestart(policy string, err error) bool {
	switch policy {
	case "always":
		return true
	case "never":
		return false
	case "on-failure", "":
		return err != nil
	default:
		return true
	}
}

func backoffDelay(attempt int) time.Duration {
	delay := time.Duration(math.Pow(2, float64(attempt))) * backoffBase
	if delay > backoffMax {
		delay = backoffMax
	}
	return delay
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
