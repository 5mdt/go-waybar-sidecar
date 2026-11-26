package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	origKillFn := killFn
	t.Cleanup(func() { killFn = origKillFn })

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

func TestAcquireInstanceLock_Exclusive(t *testing.T) {
	setupPidTest(t)

	f, _, err := acquireInstanceLock()
	if err != nil || f == nil {
		t.Fatalf("first acquireInstanceLock() = %v, %v; want lock", f, err)
	}
	defer f.Close()

	got, err := os.ReadFile(filepath.Join(pidDir, "waybar-sidecar.lock"))
	if err != nil || strings.TrimSpace(string(got)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock file = %q, %v; want own pid %d", got, err, os.Getpid())
	}

	// A second, independent acquisition (separate fd) must see the lock as
	// held, report the holder, and not error or clobber the file.
	f2, holder, err := acquireInstanceLock()
	if err != nil || f2 != nil {
		t.Fatalf("second acquireInstanceLock() = %v, %v; want nil, nil", f2, err)
	}
	if holder != os.Getpid() {
		t.Errorf("holder = %d, want %d", holder, os.Getpid())
	}
}

func TestTerminatePIDs_ExitsAfterSIGTERM(t *testing.T) {
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

	terminatePIDs([]int{4242})

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

func TestTerminatePIDs_EscalatesToSIGKILL(t *testing.T) {
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

	terminatePIDs([]int{4242})

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

func TestEnsureSingleInstance_FreeLock(t *testing.T) {
	setupPidTest(t)

	signaled := false
	killFn = func(pid int, sig syscall.Signal) error {
		signaled = true
		return nil
	}

	ensureSingleInstance()

	if signaled {
		t.Error("expected no signals sent when the lock is free")
	}
	if instanceLock == nil {
		t.Error("expected the instance lock to be acquired")
	}
}

// holdLock takes the lock file through an independent fd and writes content
// into it, like another running instance would.
func holdLock(t *testing.T, content string) *os.File {
	t.Helper()
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fd, err := os.OpenFile(filepath.Join(pidDir, "waybar-sidecar.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fd.Close() })
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := fd.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return fd
}

func TestEnsureSingleInstance_TakesOverHolder(t *testing.T) {
	setupPidTest(t)
	fd := holdLock(t, "4242\n")

	var mu sync.Mutex
	var termed []int
	killFn = func(pid int, sig syscall.Signal) error {
		mu.Lock()
		defer mu.Unlock()
		if sig == syscall.SIGTERM {
			termed = append(termed, pid)
			fd.Close() // the incumbent exits and releases the lock
		}
		if sig == 0 && len(termed) > 0 {
			return syscall.ESRCH
		}
		return nil
	}

	ensureSingleInstance()

	mu.Lock()
	defer mu.Unlock()
	if len(termed) != 1 || termed[0] != 4242 {
		t.Errorf("SIGTERM'd pids = %v, want [4242]", termed)
	}
	if instanceLock == nil {
		t.Fatal("expected the instance lock to be acquired")
	}
	got, _ := os.ReadFile(filepath.Join(pidDir, "waybar-sidecar.lock"))
	if strings.TrimSpace(string(got)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("lock file = %q, want own pid", got)
	}
}

func TestEnsureSingleInstance_ProceedsWhenLockBusy(t *testing.T) {
	buf := setupPidTest(t)
	holdLock(t, "")
	killFn = func(pid int, sig syscall.Signal) error { return nil }

	ensureSingleInstance()

	if !strings.Contains(buf.String(), "lock") {
		t.Errorf("expected a log mentioning the lock, got: %s", buf.String())
	}
	if instanceLock != nil {
		t.Error("expected no lock to be held")
	}
}
