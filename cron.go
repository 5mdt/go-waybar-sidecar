package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

// jobExec runs a shell command and returns its stdout. The command runs in its
// own process group; cancelling ctx SIGKILLs the whole group so a hung job
// cannot block a restart. Overridable for testing (mirrors sendNotification
// in notify.go).
var jobExec = func(ctx context.Context, command string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// jobNow returns the current time. Overridable for testing.
var jobNow = time.Now

// schedule is satisfied by both intervalSchedule and the cron.Schedule
// returned by robfig/cron's parser.
type schedule interface {
	Next(time.Time) time.Time
}

// intervalSchedule fires every d duration.
type intervalSchedule struct{ d time.Duration }

func (s intervalSchedule) Next(t time.Time) time.Time { return t.Add(s.d) }

// parseSchedule validates that exactly one of j.Every or j.Schedule is set
// and returns the corresponding schedule.
func parseSchedule(j Job) (schedule, error) {
	hasEvery := j.Every != ""
	hasCron := j.Schedule != ""
	switch {
	case hasEvery && hasCron:
		return nil, fmt.Errorf("job %q: only one of 'every' or 'schedule' may be set", j.Name)
	case !hasEvery && !hasCron:
		return nil, fmt.Errorf("job %q: one of 'every' or 'schedule' must be set", j.Name)
	case hasEvery:
		d, err := time.ParseDuration(j.Every)
		if err != nil {
			return nil, fmt.Errorf("job %q: invalid 'every' value %q: %w", j.Name, j.Every, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("job %q: 'every' must be positive, got %q", j.Name, j.Every)
		}
		return intervalSchedule{d: d}, nil
	default:
		s, err := cron.ParseStandard(j.Schedule)
		if err != nil {
			return nil, fmt.Errorf("job %q: invalid 'schedule' value %q: %w", j.Name, j.Schedule, err)
		}
		return s, nil
	}
}

// runJob runs j on schedule s until ctx is cancelled.
// A single job never overlaps itself: the next tick is computed after the
// previous run finishes.
func runJob(ctx context.Context, j Job, s schedule) {
	for {
		timer := time.NewTimer(max(0, s.Next(jobNow()).Sub(jobNow())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		runOnce(ctx, j)

		if ctx.Err() != nil {
			return
		}
	}
}

// runOnce executes j once. On failure it logs the error (with stderr) and
// leaves the output file untouched; on success it writes trimmed stdout to
// j.Output atomically.
func runOnce(ctx context.Context, j Job) {
	outPath := expandHome(j.Output)
	out, err := jobExec(ctx, j.Command)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		attrs := []any{"name", j.Name, "err", err}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			attrs = append(attrs, "stderr", strings.TrimSpace(string(exitErr.Stderr)))
		}
		if j.Output != "" {
			attrs = append(attrs, notifyOpen("Show output", outPath))
		}
		logger.Warn("job failed", attrs...)
		return
	}
	logger.Debug("job ran", "name", j.Name)

	if j.Output == "" {
		return
	}
	data := []byte(strings.TrimRight(string(out), "\n"))
	if err := writeFileAtomic(outPath, data); err != nil {
		logger.Warn("job output: cannot write file", "name", j.Name, "err", err)
	}
}

// jobRunner owns the lifecycle of all running job goroutines.
// restart is safe to call concurrently (called from both run() and the
// watcher reload goroutine).
type jobRunner struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	current []Job
}

// restart stops the current job set (draining in-flight runs) and starts
// fresh goroutines for jobs. Invalid jobs are logged and skipped. It does
// nothing when jobs equals the running set, so unrelated reloads don't reset
// schedules.
func (jr *jobRunner) restart(jobs []Job) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

	if jr.cancel != nil && slices.Equal(jr.current, jobs) {
		return
	}
	jr.current = slices.Clone(jobs)

	// Cancel and drain the previous set.
	if jr.cancel != nil {
		jr.cancel()
		jr.wg.Wait()
	}

	ctx, cancel := context.WithCancel(context.Background())
	jr.cancel = cancel

	for _, j := range jobs {
		s, err := parseSchedule(j)
		if err != nil {
			logger.Warn("skipping invalid job", "err", err)
			continue
		}
		jr.wg.Add(1)
		go func(jj Job, ss schedule) {
			defer jr.wg.Done()
			runJob(ctx, jj, ss)
		}(j, s)
	}
}

// jobs is the package-level job runner.
var jobs jobRunner
