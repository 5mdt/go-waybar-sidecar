package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// setupPidTest redirects the logger to a buffer, overrides pidDir to a temp
// directory, shrinks the timing vars, and restores everything (including
// instanceLock) on cleanup. Mirrors setupWaybarTest in waybar_test.go.
func setupPidTest(t *testing.T) *syncBuffer {
	t.Helper()
	dir := t.TempDir()

	origPidDir := pidDir
	pidDir = dir
	t.Cleanup(func() { pidDir = origPidDir })

	buf := &syncBuffer{}
	origLogger := logger
	logger = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { logger = origLogger })

	origGrace, origPoll := sidecarStopGrace, sidecarStopPoll
	sidecarStopGrace = 50 * time.Millisecond
	sidecarStopPoll = 5 * time.Millisecond
	t.Cleanup(func() { sidecarStopGrace, sidecarStopPoll = origGrace, origPoll })

	origKillFn, origSidecarPIDsFn := killFn, sidecarPIDsFn
	t.Cleanup(func() { killFn, sidecarPIDsFn = origKillFn, origSidecarPIDsFn })

	t.Cleanup(func() {
		if instanceLock != nil {
			instanceLock.Close()
			instanceLock = nil
		}
	})

	return buf
}

func TestIsRealProc_Self(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !isRealProc(os.Getpid(), filepath.Base(exe)) {
		t.Error("expected isRealProc(self, own exe basename) = true")
	}
	if isRealProc(os.Getpid(), "/waybar") {
		t.Error("expected isRealProc(self, /waybar) = false (test binary isn't waybar)")
	}
}

func TestIsRealProc_NonexistentPID(t *testing.T) {
	if isRealProc(1<<30, "/go-waybar-sidecar") {
		t.Error("expected isRealProc(nonexistent pid) = false")
	}
}

func TestSidecarPIDs_ExcludesSelf(t *testing.T) {
	for _, pid := range sidecarPIDs() {
		if pid == os.Getpid() {
			t.Errorf("sidecarPIDs() included own pid %d", pid)
		}
	}
}

func TestAcquireInstanceLock_Exclusive(t *testing.T) {
	setupPidTest(t)

	locked, err := acquireInstanceLock()
	if err != nil {
		t.Fatalf("first acquireInstanceLock() error = %v", err)
	}
	if !locked {
		t.Fatal("first acquireInstanceLock() = false, want true")
	}

	// A second, independent acquisition attempt on the same path (separate
	// fd) must see the lock as held, not error.
	path := filepath.Join(pidDir, "waybar-sidecar.lock")
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Errorf("expected second flock to be EWOULDBLOCK, got %v", err)
	}
}

func TestTerminateSidecars_ExitsAfterSIGTERM(t *testing.T) {
	setupPidTest(t)

	var mu sync.Mutex
	var signals []int
	gone := false

	killFn = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		signals = append(signals, int(sig))
		if sig == syscall.SIGTERM {
			gone = true
			return nil
		}
		if sig == 0 {
			if gone {
				return syscall.ESRCH
			}
			return nil
		}
		return nil
	}

	terminateSidecars([]int{4242})

	mu.Lock()
	defer mu.Unlock()
	for _, s := range signals {
		if s == int(syscall.SIGKILL) {
			t.Errorf("SIGKILL sent, want none: signals = %v", signals)
		}
	}
	if len(signals) == 0 || signals[0] != int(syscall.SIGTERM) {
		t.Errorf("expected SIGTERM first, got signals = %v", signals)
	}
}

func TestTerminateSidecars_EscalatesToSIGKILL(t *testing.T) {
	buf := setupPidTest(t)

	var mu sync.Mutex
	var signals []int

	killFn = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		signals = append(signals, int(sig))
		// Always alive: liveness probes (sig==0) succeed, SIGKILL succeeds,
		// process never reports gone.
		return nil
	}

	terminateSidecars([]int{4242})

	mu.Lock()
	defer mu.Unlock()
	hasSigterm, hasSigkill := false, false
	for _, s := range signals {
		if s == int(syscall.SIGTERM) {
			hasSigterm = true
		}
		if s == int(syscall.SIGKILL) {
			hasSigkill = true
		}
	}
	if !hasSigterm {
		t.Error("expected SIGTERM to have been sent")
	}
	if !hasSigkill {
		t.Errorf("expected SIGKILL escalation, got signals = %v", signals)
	}
	if !bytes.Contains([]byte(buf.String()), []byte("SIGKILL")) {
		t.Errorf("expected a log mentioning SIGKILL, got: %s", buf.String())
	}
}

func TestEnsureSingleInstance_NoOthers(t *testing.T) {
	setupPidTest(t)

	sidecarPIDsFn = func() []int { return nil }

	signaled := false
	killFn = func(pid int, sig syscall.Signal) error {
		if sig != 0 {
			signaled = true
		}
		return nil
	}

	ensureSingleInstance()

	if signaled {
		t.Error("expected no signals sent when no other instances are running")
	}
	if instanceLock == nil {
		t.Error("expected the instance lock to be acquired")
	}
}

func TestEnsureSingleInstance_TerminatesMultipleStrays(t *testing.T) {
	setupPidTest(t)

	sidecarPIDsFn = func() []int { return []int{111, 222} }

	var mu sync.Mutex
	termed := map[int]bool{}
	killFn = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		if sig == syscall.SIGTERM {
			termed[pid] = true
		}
		if sig == 0 {
			return syscall.ESRCH // report gone immediately after SIGTERM
		}
		return nil
	}

	ensureSingleInstance()

	mu.Lock()
	defer mu.Unlock()
	if !termed[111] || !termed[222] {
		t.Errorf("expected both stray pids to be SIGTERM'd, got %v", termed)
	}
}

func TestEnsureSingleInstance_ProceedsWhenLockBusy(t *testing.T) {
	buf := setupPidTest(t)

	sidecarPIDsFn = func() []int { return nil }
	killFn = func(pid int, sig syscall.Signal) error { return nil }

	// Hold the lock via an independent fd for the whole test.
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pidDir, "waybar-sidecar.lock")
	fd, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	ensureSingleInstance()

	if !bytes.Contains([]byte(buf.String()), []byte("lock")) {
		t.Errorf("expected a log mentioning the lock, got: %s", buf.String())
	}
}
