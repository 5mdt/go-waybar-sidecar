package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
