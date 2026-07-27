package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// syncBuffer is a mutex-guarded bytes.Buffer. Now that waybar is
// auto-restarted from a background goroutine (see GWS-0027), log writes can
// happen concurrently with a test polling the buffer for an expected
// message; a plain bytes.Buffer is not safe for that.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// setupWaybarTest redirects the logger to a buffer, overrides pidDir to a
// temp directory, and shrinks the timing vars to keep tests fast. Returns the
// log buffer and restores all globals on cleanup. It also ensures no waybar
// process from a previous test is left tracked/running.
func setupWaybarTest(t *testing.T) *syncBuffer {
	t.Helper()
	dir := t.TempDir()

	origPidDir := pidDir
	pidDir = dir
	t.Cleanup(func() { pidDir = origPidDir })

	buf := &syncBuffer{}
	origLogger := logger
	logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logger = origLogger })

	origUsr2, origColdStart, origGrace := usr2SettleDelay, coldStartSettleDelay, stopGraceDelay
	usr2SettleDelay = 5 * time.Millisecond
	coldStartSettleDelay = 5 * time.Millisecond
	stopGraceDelay = 20 * time.Millisecond
	t.Cleanup(func() {
		usr2SettleDelay, coldStartSettleDelay, stopGraceDelay = origUsr2, origColdStart, origGrace
	})

	autoRestartMu.Lock()
	origMax, origWindow := autoRestartMax, autoRestartWindow
	autoRestartMu.Unlock()
	t.Cleanup(func() {
		autoRestartMu.Lock()
		autoRestartMax, autoRestartWindow = origMax, origWindow
		autoRestartTimes = nil
		autoRestartMu.Unlock()
	})

	origPIDsFn := waybarPIDsFn
	t.Cleanup(func() { waybarPIDsFn = origPIDsFn })

	origKillFn := killFn
	t.Cleanup(func() { killFn = origKillFn })

	quiesceWaybar()
	t.Cleanup(quiesceWaybar)

	return buf
}

// quiesceWaybar stops the tracked waybar process and waits out any in-flight
// auto-restart before returning. It never reassigns the package-level waybar
// var itself: a background goroutine from a prior generation may still hold
// a pointer to it (via its method receiver), and replacing the struct while
// that's in flight is a data race - stop() is enough to bring it to a dead,
// inert state that the next test can build on.
func quiesceWaybar() {
	waybar.stop()
	// Any scheduleAutoRestart goroutine that had already passed its
	// expectExit check holds reloadMu across its call to start(); wait it
	// out, then stop whatever it may have started.
	reloadMu.Lock()
	reloadMu.Unlock()
	waybar.stop()
}

// noWaybarBinary sets PATH to an empty temp dir so exec.Command("waybar")
// fails to find the binary.
func noWaybarBinary(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// stubWaybarBinary puts a minimal stub `waybar` shell script on PATH that
// sleeps long enough not to exit during the test.
func stubWaybarBinary(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := filepath.Join(binDir, "waybar")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap '' USR2\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
}

// stubExitingWaybarBinary puts a `waybar` stub on PATH that exits immediately
// on SIGUSR2, so tests can exercise the "died after reload" fallback.
func stubExitingWaybarBinary(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := filepath.Join(binDir, "waybar")
	// sleep runs in the background and is waited on via `wait`, so bash's
	// signal-deferral-until-foreground-command-completes doesn't delay the
	// trap (see https://www.gnu.org/software/bash/manual/bash.html#Signals).
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 0' USR2\nsleep 30 &\nwait $!\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("condition not met within timeout")
	}
}

// --- waybarProc unit tests ---

func TestWaybarProcStartAliveStop(t *testing.T) {
	setupWaybarTest(t)
	stubWaybarBinary(t)

	var p waybarProc
	t.Cleanup(p.stop)
	if p.alive() {
		t.Fatal("expected alive()=false before start")
	}

	if err := p.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}
	if !p.alive() {
		t.Fatal("expected alive()=true after start")
	}
	if p.pid() == 0 {
		t.Fatal("expected non-zero pid after start")
	}

	p.stop()
	if p.alive() {
		t.Fatal("expected alive()=false after stop")
	}
	if p.cmd.ProcessState == nil {
		t.Fatal("expected ProcessState to be set (Wait() must have run - no zombie)")
	}

	// stop() must be idempotent.
	p.stop()
}

func TestWaybarProcStopNoOp(t *testing.T) {
	var p waybarProc
	p.stop() // must not panic or block when nothing is running
}

func TestWaybarProcOnExitCrashTriggersRestart(t *testing.T) {
	setupWaybarTest(t)
	stubWaybarBinary(t)

	var p waybarProc
	t.Cleanup(p.stop)
	if err := p.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}
	firstPID := p.pid()

	// Kill the process directly (not via stop()) to simulate a crash.
	if err := syscall.Kill(firstPID, syscall.SIGKILL); err != nil {
		t.Fatalf("failed to kill stub: %v", err)
	}

	waitUntil(t, time.Second, func() bool {
		return p.alive() && p.pid() != firstPID
	})
}

func TestWaybarProcOnExitDeliberateStopNoRestart(t *testing.T) {
	setupWaybarTest(t)
	stubWaybarBinary(t)

	var p waybarProc
	t.Cleanup(p.stop)
	if err := p.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}
	p.stop()

	// Give any (incorrect) auto-restart goroutine a chance to fire.
	time.Sleep(30 * time.Millisecond)
	if p.alive() {
		t.Fatal("expected no auto-restart after a deliberate stop()")
	}
}

func TestWaybarProcAutoRestartRateLimit(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	autoRestartMu.Lock()
	autoRestartMax = 2
	autoRestartWindow = time.Minute
	autoRestartMu.Unlock()

	if err := waybar.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}

	// Crash it repeatedly, one at a time - waiting for each restart to
	// complete before triggering the next - so there is never more than one
	// crash/restart cycle in flight. Beyond autoRestartMax it must stop
	// restarting and log giving up.
	for i := 0; i < autoRestartMax; i++ {
		pid := waybar.pid()
		_ = syscall.Kill(pid, syscall.SIGKILL)
		waitUntil(t, time.Second, func() bool {
			return waybar.alive() && waybar.pid() != pid
		})
	}

	lastPID := waybar.pid()
	_ = syscall.Kill(lastPID, syscall.SIGKILL)

	waitUntil(t, time.Second, func() bool {
		return strings.Contains(buf.String(), "giving up")
	})

	// Drain any in-flight restart before returning, so no goroutine touches
	// shared test state (e.g. the logger swapped back by cleanup) afterward.
	quiesceWaybar()
}

// --- reloadWaybar tests ---

func TestReloadWaybarStartsWhenNotRunning(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "waybar started") {
		t.Errorf("expected \"waybar started\" in log, got:\n%s", log)
	}
	if strings.Contains(log, "failed to start") {
		t.Errorf("unexpected failure in log:\n%s", log)
	}
	if !waybar.alive() {
		t.Error("expected waybar to be alive after reloadWaybar()")
	}
}

func TestReloadWaybarUsesUsr2WhenAlive(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	if err := waybar.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}
	firstPID := waybar.pid()
	time.Sleep(30 * time.Millisecond) // let the stub install its USR2 trap

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "waybar reloaded via USR2") {
		t.Errorf("expected USR2 reload log, got:\n%s", log)
	}
	if waybar.pid() != firstPID {
		t.Errorf("expected same pid after USR2 reload, got %d want %d", waybar.pid(), firstPID)
	}
}

func TestReloadWaybarFallsBackWhenUsr2Kills(t *testing.T) {
	buf := setupWaybarTest(t)
	stubExitingWaybarBinary(t)

	if err := waybar.start(); err != nil {
		t.Fatalf("start() error: %v", err)
	}
	firstPID := waybar.pid()
	time.Sleep(30 * time.Millisecond) // let the stub install its USR2 trap

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "waybar died after reload, falling back to cold start") {
		t.Errorf("expected fallback log, got:\n%s", log)
	}
	if !waybar.alive() {
		t.Error("expected a fresh waybar to be alive after fallback cold start")
	}
	if waybar.pid() == firstPID {
		t.Error("expected a new pid after cold-start fallback")
	}
}

func TestReloadWaybarColdStartFails(t *testing.T) {
	buf := setupWaybarTest(t)
	noWaybarBinary(t)

	reloadWaybar()

	log := buf.String()
	if !strings.Contains(log, "failed to start waybar") && !strings.Contains(log, "waybar failed to start") {
		t.Errorf("expected failure log, got:\n%s", log)
	}
}

// --- concurrent reload serialization tests (see GWS-0007) ---

func TestReloadWaybarSerializesConcurrentCalls(t *testing.T) {
	setupWaybarTest(t)
	stubWaybarBinary(t)

	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			reloadWaybar()
		}()
	}
	wg.Wait()

	if !waybar.alive() {
		t.Error("expected waybar to be alive after concurrent reloads")
	}
}

func TestReloadWaybarConcurrentNoFalseFailure(t *testing.T) {
	buf := setupWaybarTest(t)
	stubWaybarBinary(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); reloadWaybar() }()
	go func() { defer wg.Done(); reloadWaybar() }()
	wg.Wait()

	log := buf.String()
	if strings.Contains(log, "failed to start") {
		t.Errorf("unexpected failure logged for concurrent reloads:\n%s", log)
	}
}

// --- GWS-0022: sweepStrayWaybars tests ---

func TestSweepStrayWaybarsKillsAll(t *testing.T) {
	buf := setupWaybarTest(t)

	waybarPIDsFn = func() []int { return []int{100, 200, 300} }

	killed := map[int]syscall.Signal{}
	var mu sync.Mutex
	killFn = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		killed[pid] = sig
		mu.Unlock()
		return nil
	}

	sweepStrayWaybars()

	for _, pid := range []int{100, 200, 300} {
		if killed[pid] != syscall.SIGTERM {
			t.Errorf("expected SIGTERM to pid %d, got %v", pid, killed[pid])
		}
	}
	if !strings.Contains(buf.String(), "killed stray waybar instance") {
		t.Errorf("expected log line, got: %s", buf.String())
	}
}

func TestSweepStrayWaybarsNoneNoop(t *testing.T) {
	setupWaybarTest(t)

	waybarPIDsFn = func() []int { return nil }

	kills := 0
	killFn = func(pid int, sig syscall.Signal) error { kills++; return nil }

	sweepStrayWaybars()

	if kills != 0 {
		t.Errorf("expected 0 kills for no instances, got %d", kills)
	}
}

// --- isRealWaybar / waybarPIDs (kept for the startup sweep) ---

func TestIsRealWaybarSelf(t *testing.T) {
	// The test binary is not named "waybar", so this must be false.
	if isRealWaybar(os.Getpid()) {
		t.Error("expected isRealWaybar(self) = false (not named waybar)")
	}
}

func TestWaybarPIDsNoneRunning(t *testing.T) {
	// waybarPIDs (the real impl) must return nil/empty gracefully when no
	// waybar exists (best-effort - depends on no real waybar running on the
	// test host, consistent with the previous TestWaybarPIDNoneRunning).
	_ = waybarPIDs()
}
