// Package cron implements a background scheduler that fires agent tasks at
// configured intervals and delivers the result to a target push address.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chickenzord/zlaw/internal/config"
	"github.com/chickenzord/zlaw/internal/push"
)

// AgentRunner is the interface the Scheduler uses to execute cron tasks.
// It mirrors the agent.Agent.Run signature but returns only the response text.
type AgentRunner interface {
	Run(ctx context.Context, sessionID, input, systemPrompt string) (string, error)
}

// Scheduler runs cron jobs on a per-minute tick loop. Jobs are hot-reloaded
// via Reload; per-job mutexes prevent duplicate concurrent runs.
type Scheduler struct {
	runner      AgentRunner
	pushReg     *push.Registry
	sysPromptFn func() string
	logger      *slog.Logger

	mu       sync.RWMutex
	jobs     []config.CronJobConfig
	runLocks sync.Map // map[string]*sync.Mutex, keyed by job ID
}

// NewScheduler creates a Scheduler. sysPromptFn is called at run time to get
// the current system prompt so hot-reloaded personality changes apply.
func NewScheduler(
	runner AgentRunner,
	pushReg *push.Registry,
	sysPromptFn func() string,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		runner:      runner,
		pushReg:     pushReg,
		sysPromptFn: sysPromptFn,
		logger:      logger,
	}
}

// Reload atomically replaces the active job list. Safe to call from any goroutine.
func (s *Scheduler) Reload(jobs []config.CronJobConfig) {
	s.mu.Lock()
	s.jobs = jobs
	s.mu.Unlock()
	s.logger.Info("cron: jobs reloaded", "count", len(jobs))
}

// Run starts the per-minute tick loop aligned to wall-clock minute boundaries.
// Blocks until ctx is cancelled; call in a goroutine.
func (s *Scheduler) Run(ctx context.Context) {
	// Align to the next whole minute so ticks fire at :00 seconds.
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Until(next)):
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			s.tick(ctx, t)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, t time.Time) {
	s.mu.RLock()
	jobs := make([]config.CronJobConfig, len(s.jobs))
	copy(jobs, s.jobs)
	s.mu.RUnlock()

	for _, job := range jobs {
		matches, err := matchesCron(job.Schedule, t)
		if err != nil {
			s.logger.Error("cron: invalid schedule, skipping job",
				"job_id", job.ID, "schedule", job.Schedule, "err", err)
			continue
		}
		if matches {
			go s.runJob(ctx, job, t)
		}
	}
}

func (s *Scheduler) runJob(ctx context.Context, job config.CronJobConfig, firedAt time.Time) {
	// Per-job run lock: skip if a previous instance is still running.
	mu, _ := s.runLocks.LoadOrStore(job.ID, &sync.Mutex{})
	jobMu := mu.(*sync.Mutex)
	if !jobMu.TryLock() {
		s.logger.Warn("cron: previous run still active, skipping", "job_id", job.ID)
		return
	}
	defer jobMu.Unlock()

	log := s.logger.With("job_id", job.ID, "fired_at", firedAt.Format(time.RFC3339))
	log.Info("cron: job started")

	sessionID := fmt.Sprintf("cron:%s:%d", job.ID, firedAt.Unix())
	text, err := s.runner.Run(ctx, sessionID, job.Task, s.sysPromptFn())
	if err != nil {
		log.Error("cron: job execution failed", "err", err)
		return
	}

	if job.Target == "" {
		log.Warn("cron: no target configured, delivery skipped")
		return
	}

	if err := s.pushReg.Push(ctx, job.Target, text); err != nil {
		log.Error("cron: push failed", "target", job.Target, "err", err)
		return
	}
	log.Info("cron: job delivered", "target", job.Target)
}
