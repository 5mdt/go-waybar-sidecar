package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// jobExec runs a shell command and returns its combined stdout.
// Overridable for testing (mirrors sendNotification in notify.go).
var jobExec = func(command string) ([]byte, error) {
	return exec.Command("sh", "-c", command).Output()
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
		next := s.Next(jobNow())
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		out, err := jobExec(j.Command)
		outPath := expandHome(j.Output)
		if err != nil {
			if j.Output != "" {
				logger.Warn("job failed", "name", j.Name, "err", err, notifyOpen("Show output", outPath))
			} else {
				logger.Warn("job failed", "name", j.Name, "err", err)
			}
		} else {
			logger.Debug("job ran", "name", j.Name)
		}

		if j.Output != "" {
			if mkErr := os.MkdirAll(filepath.Dir(outPath), 0o755); mkErr != nil {
				logger.Warn("job output: cannot create dir", "name", j.Name, "err", mkErr)
			} else if wErr := os.WriteFile(outPath, []byte(strings.TrimRight(string(out), "\n")), 0o644); wErr != nil {
				logger.Warn("job output: cannot write file", "name", j.Name, "err", wErr)
			}
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// jobRunner owns the lifecycle of all running job goroutines.
// restart is safe to call concurrently (called from both run() and the
// watcher reload goroutine).
type jobRunner struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// restart stops the current job set (draining in-flight runs) and starts
// fresh goroutines for jobs. Invalid jobs are logged and skipped.
func (jr *jobRunner) restart(jobs []Job) {
	jr.mu.Lock()
	defer jr.mu.Unlock()

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
