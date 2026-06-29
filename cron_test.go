package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseSchedule_Interval verifies valid every: values.
func TestParseSchedule_Interval(t *testing.T) {
	j := Job{Name: "t", Every: "5m", Command: "true"}
	s, err := parseSchedule(j)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Unix(0, 0)
	got := s.Next(now)
	want := now.Add(5 * time.Minute)
	if got != want {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

// TestParseSchedule_Cron verifies valid schedule: values.
func TestParseSchedule_Cron(t *testing.T) {
	j := Job{Name: "t", Schedule: "*/5 * * * *", Command: "true"}
	s, err := parseSchedule(j)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "*/5 * * * *" fires at :00, :05, :10, ... so Next after 00:01 is 00:05
	ref := time.Date(2025, 1, 1, 0, 1, 0, 0, time.UTC)
	got := s.Next(ref)
	want := time.Date(2025, 1, 1, 0, 5, 0, 0, time.UTC)
	if got != want {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

// TestParseSchedule_BothSet errors when both fields are given.
func TestParseSchedule_BothSet(t *testing.T) {
	j := Job{Name: "t", Every: "5m", Schedule: "* * * * *", Command: "true"}
	if _, err := parseSchedule(j); err == nil {
		t.Fatal("expected error when both every and schedule are set")
	}
}

// TestParseSchedule_NeitherSet errors when neither field is given.
func TestParseSchedule_NeitherSet(t *testing.T) {
	j := Job{Name: "t", Command: "true"}
	if _, err := parseSchedule(j); err == nil {
		t.Fatal("expected error when neither every nor schedule is set")
	}
}

// TestParseSchedule_BadDuration errors on an unparseable every: value.
func TestParseSchedule_BadDuration(t *testing.T) {
	j := Job{Name: "t", Every: "notaduration", Command: "true"}
	if _, err := parseSchedule(j); err == nil {
		t.Fatal("expected error for bad duration")
	}
}

// TestParseSchedule_BadCron errors on an invalid cron expression.
func TestParseSchedule_BadCron(t *testing.T) {
	j := Job{Name: "t", Schedule: "not a cron", Command: "true"}
	if _, err := parseSchedule(j); err == nil {
		t.Fatal("expected error for bad cron expression")
	}
}

// TestIntervalScheduleNext checks intervalSchedule.Next directly.
func TestIntervalScheduleNext(t *testing.T) {
	s := intervalSchedule{d: 30 * time.Second}
	ref := time.Unix(1000, 0)
	got := s.Next(ref)
	want := ref.Add(30 * time.Second)
	if got != want {
		t.Errorf("Next = %v, want %v", got, want)
	}
}

// TestRunJob_InvokedOnTick asserts the command is called after the schedule fires.
func TestRunJob_InvokedOnTick(t *testing.T) {
	origNow := jobNow
	origExec := jobExec

	var calls atomic.Int32
	jobExec = func(_ string) ([]byte, error) {
		calls.Add(1)
		return []byte("ok"), nil
	}

	start := time.Now()
	jobNow = func() time.Time { return start }

	ctx, cancel := context.WithCancel(context.Background())

	j := Job{Name: "t", Every: "5ms", Command: "echo ok"}
	s, err := parseSchedule(j)
	if err != nil {
		cancel()
		jobNow = origNow
		jobExec = origExec
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, j, s)
	}()

	// Wait up to 200ms for at least one call.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Cancel and drain the goroutine BEFORE restoring globals.
	cancel()
	<-done
	jobNow = origNow
	jobExec = origExec

	if calls.Load() < 1 {
		t.Error("expected jobExec to be called at least once")
	}
}

// TestRunJob_CancelStopsLoop asserts context cancellation stops further calls.
func TestRunJob_CancelStopsLoop(t *testing.T) {
	origNow := jobNow
	origExec := jobExec
	defer func() { jobNow = origNow; jobExec = origExec }()

	var calls atomic.Int32
	jobExec = func(_ string) ([]byte, error) {
		calls.Add(1)
		return nil, nil
	}

	start := time.Now()
	jobNow = func() time.Time { return start }

	ctx, cancel := context.WithCancel(context.Background())

	j := Job{Name: "t", Every: "5ms", Command: "true"}
	s, _ := parseSchedule(j)

	done := make(chan struct{})
	go func() {
		runJob(ctx, j, s)
		close(done)
	}()

	// Let it fire a couple times, then cancel.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runJob did not stop after context cancellation")
	}

	snapshot := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != snapshot {
		t.Errorf("jobExec called after cancel (%d → %d)", snapshot, calls.Load())
	}
}

// TestRunJob_OutputWritten asserts stdout is written to j.Output.
func TestRunJob_OutputWritten(t *testing.T) {
	origNow := jobNow
	origExec := jobExec

	jobExec = func(_ string) ([]byte, error) {
		return []byte("  hello world\n"), nil
	}

	start := time.Now()
	jobNow = func() time.Time { return start }

	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.txt")

	ctx, cancel := context.WithCancel(context.Background())

	j := Job{Name: "t", Every: "5ms", Command: "echo hello world", Output: outFile}
	s, _ := parseSchedule(j)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runJob(ctx, j, s)
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(outFile); err == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Cancel and drain the goroutine BEFORE restoring globals.
	cancel()
	<-done
	jobNow = origNow
	jobExec = origExec

	b, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("output file not created: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "hello world") {
		t.Errorf("output file content = %q, want it to contain %q", got, "hello world")
	}
}

// TestJobRunner_RestartStopsOldSet asserts restart cancels old goroutines.
func TestJobRunner_RestartStopsOldSet(t *testing.T) {
	origNow := jobNow
	origExec := jobExec
	defer func() { jobNow = origNow; jobExec = origExec }()

	var mu sync.Mutex
	callsByName := map[string]int{}
	jobExec = func(cmd string) ([]byte, error) {
		mu.Lock()
		callsByName[cmd]++
		mu.Unlock()
		return nil, nil
	}

	start := time.Now()
	jobNow = func() time.Time { return start }

	var jr jobRunner

	first := []Job{{Name: "first", Every: "5ms", Command: "first"}}
	jr.restart(first)

	// Let first job fire a few times.
	time.Sleep(40 * time.Millisecond)

	second := []Job{{Name: "second", Every: "5ms", Command: "second"}}
	jr.restart(second)

	// Record calls immediately after restart.
	mu.Lock()
	firstCount := callsByName["first"]
	mu.Unlock()

	// Let second set run for a while.
	time.Sleep(40 * time.Millisecond)

	mu.Lock()
	firstCountAfter := callsByName["first"]
	secondCount := callsByName["second"]
	mu.Unlock()

	// First job must have stopped — no new calls after restart.
	if firstCountAfter != firstCount {
		t.Errorf("first job fired %d more times after restart", firstCountAfter-firstCount)
	}
	// Second job must have fired at least once.
	if secondCount < 1 {
		t.Errorf("second job did not fire after restart (calls = %d)", secondCount)
	}

	// Cleanup.
	jr.restart(nil)
}
