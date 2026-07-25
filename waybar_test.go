package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// setupWaybarTest redirects the logger to a buffer, overrides pidDir to a
// temp directory, and stubs the timing vars to keep tests fast. Returns the
// log buffer and restores all globals on cleanup.
func setupWaybarTest(t *testing.T) *bytes.Buffer {
	t.Helper()
	dir := t.TempDir()

	origPidDir := pidDir
	pidDir = dir
	t.Cleanup(func() { pidDir = origPidDir })

	var buf bytes.Buffer
	origLogger := logger
	logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logger = origLogger })

	origAttempts, origPollSleep, origKillSleep := coldStartPollAttempts, coldStartPollSleep, coldStartKillSleep
	coldStartPollAttempts = 2
	coldStartPollSleep = 5 * time.Millisecond
	coldStartKillSleep = 2 * time.Millisecond
	t.Cleanup(func() {
		coldStartPollAttempts = origAttempts
		coldStartPollSleep = origPollSleep
		coldStartKillSleep = origKillSleep
	})

	origFn := waybarPIDFn
	t.Cleanup(func() { waybarPIDFn = origFn })

	origPIDsFn := waybarPIDsFn
	t.Cleanup(func() { waybarPIDsFn = origPIDsFn })

	origStartTimeFn := procStartTimeFn
	t.Cleanup(func() { procStartTimeFn = origStartTimeFn })

	origKillFn := killFn
	t.Cleanup(func() { killFn = origKillFn })

	return &buf
}

// noWaybarBinary sets PATH to an empty temp dir so exec.Command("waybar")
// fails to find the binary.
func noWaybarBinary(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// stubWaybarBinary puts a minimal stub `waybar` shell script on PATH that
// sleeps long enough not to exit during the test. Because pgrep -x waybar
// matches process *name* (not exe path) and a shell script runs as `sh`,
// callers should inject waybarPIDFn instead of relying on pgrep finding it.
func stubWaybarBinary(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := filepath.Join(binDir, "waybar")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
}

func TestReloadWaybarColdStartSucceedsInPoll(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	// First call: initial SIGUSR2 check → 0 (not running yet).
	// Subsequent calls: poll returns a valid PID on the first attempt.
	call := 0
	waybarPIDFn = func() int {
		call++
		if call == 1 {
			return 0 // initial check: not running
		}
		return 42 // first poll: found
	}

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "waybar started") {
		t.Errorf("expected \"waybar started\" in log, got:\n%s", log)
	}
	if strings.Contains(log, "waybar failed to start within 2s") {
		t.Errorf("unexpected failure in log:\n%s", log)
	}
	// waybar.pid must NOT be written.
	if _, err := os.Stat(filepath.Join(pidDir, "waybar.pid")); !os.IsNotExist(err) {
		t.Errorf("waybar.pid should not be written, err=%v", err)
	}
}

func TestReloadWaybarFinalCheckSucceeds(t *testing.T) {
	// Waybar only becomes detectable after the poll window — the final check
	// must find it and return without logging an error.
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	call := 0
	waybarPIDFn = func() int {
		call++
		// call 1: initial SIGUSR2 check → 0
		// calls 2..1+coldStartPollAttempts: poll → 0 (still not visible)
		// call 2+coldStartPollAttempts: final check → 42
		if call <= 1+coldStartPollAttempts {
			return 0
		}
		return 42
	}

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "waybar started") {
		t.Errorf("expected \"waybar started\" in log, got:\n%s", log)
	}
	if strings.Contains(log, "waybar failed to start within 2s") {
		t.Errorf("unexpected failure in log:\n%s", log)
	}
}

func TestReloadWaybarColdStartFails(t *testing.T) {
	// waybar binary not found and pgrep never returns a PID → error logged.
	buf := setupWaybarTest(t)
	noWaybarBinary(t)

	waybarPIDFn = func() int { return 0 }

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "failed to start waybar") && !strings.Contains(log, "waybar failed to start within 2s") {
		t.Errorf("expected failure log, got:\n%s", log)
	}
}

func TestWaybarPIDNoneRunning(t *testing.T) {
	// waybarPID (the real impl) must return 0 gracefully when no waybar exists.
	if pid := waybarPID(); pid != 0 {
		t.Errorf("waybarPID() = %d, want 0 when waybar is not running", pid)
	}
}

// --- concurrent reload serialization tests (see GWS-0007) ---
//
// Rationale: timer.Stop() in watcher.go cannot cancel a timer whose callback
// has already fired, so two reload(cfg) goroutines can call reloadWaybar()
// concurrently. Unguarded, they pkill/start waybar independently and can
// clobber each other's freshly-spawned process, producing a spurious
// "waybar failed to start within 2s" even though waybar ends up running.
// reloadWaybar() must serialize itself against this.

func TestReloadWaybarSerializesConcurrentCalls(t *testing.T) {
	setupWaybarTest(t)
	stubWaybarBinary(t)
	waybarPIDsFn = func() []int { return nil }

	// waybarPIDFn always reports "not running" so every invocation takes the
	// full cold-start + poll path, holding the guarded section open long
	// enough (coldStartPollAttempts x coldStartPollSleep, plus this stub's
	// own delay) for a race to show up if calls aren't serialized.
	var inFlight, maxSeen int32
	waybarPIDFn = func() int {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxSeen)
			if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
				break
			}
		}
		time.Sleep(3 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return 0
	}

	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			reloadWaybar()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxSeen); got > 1 {
		t.Errorf("reloadWaybar() calls overlapped: observed %d concurrent invocations in the guarded section, want 1", got)
	}
}

func TestReloadWaybarConcurrentNoFalseFailure(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)
	waybarPIDsFn = func() []int { return nil }

	// Model waybar becoming detectable partway through startup and staying
	// detectable (monotonic, like a real process): every check before the
	// threshold reports "not running", every check after reports "running".
	// This must hold under concurrent triggering: once waybar is up, no
	// concurrently-triggered reload may still log a failure.
	start := time.Now()
	waybarPIDFn = func() int {
		if time.Since(start) < 6*time.Millisecond {
			return 0
		}
		return 42
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); reloadWaybar() }()
	go func() { defer wg.Done(); reloadWaybar() }()
	wg.Wait()

	log := buf.String()
	if strings.Contains(log, "waybar failed to start within 2s") {
		t.Errorf("unexpected failure logged despite waybar being detectable:\n%s", log)
	}
}

// --- GWS-0022: reapDuplicateWaybars tests ---

func TestReapDuplicateWaybarsKillsOlder(t *testing.T) {
	buf := setupWaybarTest(t)

	// PID 100 started earlier (time 10), PID 200 started later (time 20).
	waybarPIDsFn = func() []int { return []int{100, 200} }
	startTimes := map[int]uint64{100: 10, 200: 20}
	procStartTimeFn = func(pid int) (uint64, error) { return startTimes[pid], nil }

	killed := map[int]syscall.Signal{}
	killFn = func(pid int, sig syscall.Signal) error {
		killed[pid] = sig
		return nil
	}

	reapDuplicateWaybars()

	if killed[100] != syscall.SIGTERM {
		t.Errorf("expected SIGTERM to older PID 100, got %v", killed[100])
	}
	if _, ok := killed[200]; ok {
		t.Errorf("newest PID 200 must not be killed")
	}
	if !strings.Contains(buf.String(), "killed older waybar instance") {
		t.Errorf("expected log line, got: %s", buf.String())
	}
}

func TestReapDuplicateWaybarsKeepsNewestNotFirst(t *testing.T) {
	setupWaybarTest(t)

	// PID 100 is the newest (time 30), but pgrep returned it first.
	waybarPIDsFn = func() []int { return []int{100, 200, 300} }
	startTimes := map[int]uint64{100: 30, 200: 10, 300: 20}
	procStartTimeFn = func(pid int) (uint64, error) { return startTimes[pid], nil }

	killed := map[int]syscall.Signal{}
	killFn = func(pid int, sig syscall.Signal) error {
		killed[pid] = sig
		return nil
	}

	reapDuplicateWaybars()

	if _, ok := killed[100]; ok {
		t.Errorf("newest PID 100 must not be killed")
	}
	if killed[200] != syscall.SIGTERM {
		t.Errorf("expected SIGTERM to PID 200, got %v", killed[200])
	}
	if killed[300] != syscall.SIGTERM {
		t.Errorf("expected SIGTERM to PID 300, got %v", killed[300])
	}
}

func TestReapDuplicateWaybarsSingleInstanceNoop(t *testing.T) {
	setupWaybarTest(t)

	waybarPIDsFn = func() []int { return []int{100} }
	procStartTimeFn = func(pid int) (uint64, error) { return 10, nil }

	kills := 0
	killFn = func(pid int, sig syscall.Signal) error { kills++; return nil }

	reapDuplicateWaybars()

	if kills != 0 {
		t.Errorf("expected 0 kills for single instance, got %d", kills)
	}
}

func TestReapDuplicateWaybarsNoneNoop(t *testing.T) {
	setupWaybarTest(t)

	waybarPIDsFn = func() []int { return nil }

	kills := 0
	killFn = func(pid int, sig syscall.Signal) error { kills++; return nil }

	reapDuplicateWaybars()

	if kills != 0 {
		t.Errorf("expected 0 kills for no instances, got %d", kills)
	}
}

func TestProcStartTimeSelf(t *testing.T) {
	st, err := procStartTime(os.Getpid())
	if err != nil {
		t.Fatalf("procStartTime(self) error: %v", err)
	}
	if st == 0 {
		t.Error("procStartTime(self) returned 0, want non-zero")
	}
}
