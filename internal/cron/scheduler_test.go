package cron

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zsomething/zlaw/internal/config"
	"github.com/zsomething/zlaw/internal/push"
)

// --- stubs ---

type stubRunner struct {
	mu     sync.Mutex
	calls  []runCall
	result string
	err    error
	delay  time.Duration
}

type runCall struct {
	sessionID string
	input     string
}

func (r *stubRunner) Run(_ context.Context, sessionID, input, _ string) (string, error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	r.mu.Lock()
	r.calls = append(r.calls, runCall{sessionID: sessionID, input: input})
	r.mu.Unlock()
	return r.result, r.err
}

func (r *stubRunner) Calls() []runCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]runCall, len(r.calls))
	copy(out, r.calls)
	return out
}

type stubPusher struct {
	mu       sync.Mutex
	messages []string
	err      error
}

func (p *stubPusher) Push(_ context.Context, _ string, message string) error {
	p.mu.Lock()
	p.messages = append(p.messages, message)
	p.mu.Unlock()
	return p.err
}

func (p *stubPusher) Messages() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.messages))
	copy(out, p.messages)
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestScheduler(runner AgentRunner, reg *push.Registry) *Scheduler {
	return NewScheduler(runner, reg, func() string { return "system" }, discardLogger())
}

// --- tick tests ---

func TestTick_NoJobs_NoRuns(t *testing.T) {
	runner := &stubRunner{result: "ok"}
	reg := push.NewRegistry()
	s := newTestScheduler(runner, reg)

	// tick with empty job list — nothing should be called
	s.tick(context.Background(), time.Now())

	if len(runner.Calls()) != 0 {
		t.Fatalf("expected 0 runner calls, got %d", len(runner.Calls()))
	}
}

func TestTick_MatchingSchedule_RunsJob(t *testing.T) {
	runner := &stubRunner{result: "done"}
	pusher := &stubPusher{}
	reg := push.NewRegistry()
	reg.Register("test", pusher)

	s := newTestScheduler(runner, reg)
	// Use "* * * * *" so any time matches.
	s.Reload([]config.CronJobConfig{
		{ID: "j1", Schedule: "* * * * *", Task: "hello", Target: "test:addr"},
	})

	// Fixed time — any minute matches.
	at := time.Date(2026, 4, 7, 8, 0, 0, 0, time.UTC)
	s.tick(context.Background(), at)

	// runJob is launched as a goroutine — wait briefly.
	time.Sleep(50 * time.Millisecond)

	calls := runner.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 runner call, got %d", len(calls))
	}
	if calls[0].input != "hello" {
		t.Errorf("expected task 'hello', got %q", calls[0].input)
	}
}

func TestTick_NonMatchingSchedule_NoRun(t *testing.T) {
	runner := &stubRunner{result: "ok"}
	reg := push.NewRegistry()
	s := newTestScheduler(runner, reg)

	// "0 3 * * *" — only matches at 03:00; use 08:00.
	s.Reload([]config.CronJobConfig{
		{ID: "j1", Schedule: "0 3 * * *", Task: "night task"},
	})

	at := time.Date(2026, 4, 7, 8, 0, 0, 0, time.UTC)
	s.tick(context.Background(), at)
	time.Sleep(20 * time.Millisecond)

	if len(runner.Calls()) != 0 {
		t.Fatalf("expected 0 runner calls for non-matching schedule, got %d", len(runner.Calls()))
	}
}

func TestTick_InvalidSchedule_NoRunNoPanic(t *testing.T) {
	runner := &stubRunner{result: "ok"}
	reg := push.NewRegistry()
	s := newTestScheduler(runner, reg)

	s.Reload([]config.CronJobConfig{
		{ID: "j1", Schedule: "not-a-cron", Task: "bad"},
	})

	// Must not panic.
	s.tick(context.Background(), time.Now())

	if len(runner.Calls()) != 0 {
		t.Fatalf("invalid schedule should produce no runs, got %d", len(runner.Calls()))
	}
}

// --- runJob tests ---

func TestRunJob_PushesResultToTarget(t *testing.T) {
	runner := &stubRunner{result: "agent response"}
	pusher := &stubPusher{}
	reg := push.NewRegistry()
	reg.Register("test", pusher)

	s := newTestScheduler(runner, reg)
	job := config.CronJobConfig{ID: "j1", Schedule: "* * * * *", Task: "ping", Target: "test:chat1"}

	s.runJob(context.Background(), job, time.Now())

	msgs := pusher.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 pushed message, got %d", len(msgs))
	}
	if msgs[0] != "agent response" {
		t.Errorf("expected 'agent response', got %q", msgs[0])
	}
}

func TestRunJob_NoTarget_SkipsPush(t *testing.T) {
	runner := &stubRunner{result: "ok"}
	pusher := &stubPusher{}
	reg := push.NewRegistry()
	reg.Register("test", pusher)

	s := newTestScheduler(runner, reg)
	job := config.CronJobConfig{ID: "j1", Schedule: "* * * * *", Task: "ping", Target: ""}

	s.runJob(context.Background(), job, time.Now())

	if len(pusher.Messages()) != 0 {
		t.Fatal("no target: push should be skipped")
	}
}

func TestRunJob_RunnerError_SkipsPush(t *testing.T) {
	runner := &stubRunner{err: errors.New("llm error")}
	pusher := &stubPusher{}
	reg := push.NewRegistry()
	reg.Register("test", pusher)

	s := newTestScheduler(runner, reg)
	job := config.CronJobConfig{ID: "j1", Schedule: "* * * * *", Task: "ping", Target: "test:chat1"}

	s.runJob(context.Background(), job, time.Now())

	if len(pusher.Messages()) != 0 {
		t.Fatal("runner error: push should be skipped")
	}
}

func TestRunJob_ConcurrentRunSkipped(t *testing.T) {
	var running atomic.Bool
	var skipped atomic.Bool

	slowRunner := &slowRunnerImpl{
		delay:   100 * time.Millisecond,
		result:  "ok",
		started: &running,
	}
	reg := push.NewRegistry()
	s := newTestScheduler(slowRunner, reg)
	job := config.CronJobConfig{ID: "j1", Schedule: "* * * * *", Task: "slow"}

	// Start the first run in a goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runJob(context.Background(), job, time.Now())
	}()

	// Wait until the first run has started.
	deadline := time.Now().Add(200 * time.Millisecond)
	for !running.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	// Try to start a second concurrent run.
	go func() {
		// If the second invocation immediately returns without calling runner,
		// record that as "skipped".
		before := len(slowRunner.Calls())
		s.runJob(context.Background(), job, time.Now())
		if len(slowRunner.Calls()) == before {
			skipped.Store(true)
		}
	}()

	wg.Wait()
	time.Sleep(20 * time.Millisecond) // let second goroutine finish

	if !skipped.Load() {
		t.Error("expected concurrent run to be skipped due to per-job lock")
	}
}

// --- Reload test ---

func TestReload_UpdatesJobList(t *testing.T) {
	runner := &stubRunner{result: "ok"}
	reg := push.NewRegistry()
	s := newTestScheduler(runner, reg)

	s.Reload([]config.CronJobConfig{
		{ID: "j1", Schedule: "* * * * *", Task: "first"},
	})

	at := time.Date(2026, 4, 7, 8, 0, 0, 0, time.UTC)
	s.tick(context.Background(), at)
	time.Sleep(30 * time.Millisecond)
	first := len(runner.Calls())

	// Reload with no jobs — tick should not run any.
	s.Reload(nil)
	s.tick(context.Background(), at)
	time.Sleep(30 * time.Millisecond)

	if len(runner.Calls()) != first {
		t.Errorf("after Reload(nil), no new runs expected; had %d before, got %d after",
			first, len(runner.Calls()))
	}
}

// slowRunnerImpl is a runner that signals when it starts.
type slowRunnerImpl struct {
	mu      sync.Mutex
	calls   []runCall
	delay   time.Duration
	result  string
	started *atomic.Bool
}

func (r *slowRunnerImpl) Run(_ context.Context, sessionID, input, _ string) (string, error) {
	r.started.Store(true)
	time.Sleep(r.delay)
	r.mu.Lock()
	r.calls = append(r.calls, runCall{sessionID: sessionID, input: input})
	r.mu.Unlock()
	return r.result, nil
}

func (r *slowRunnerImpl) Calls() []runCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]runCall, len(r.calls))
	copy(out, r.calls)
	return out
}
