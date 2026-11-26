package main

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Timing vars so tests can shrink them.
var (
	usr2SettleDelay      = 500 * time.Millisecond
	coldStartSettleDelay = 500 * time.Millisecond
	stopGraceDelay       = 2 * time.Second
)

// Auto-restart rate limit: give up after autoRestartMax restarts within
// autoRestartWindow, rather than hot-looping against a persistently crashing
// waybar. Vars so tests can shrink them.
var (
	autoRestartMax    = 5
	autoRestartWindow = time.Minute
)

// killFn and waybarPIDsFn are indirections so tests can stub them.
var (
	killFn       = syscall.Kill
	waybarPIDsFn = waybarPIDs
)

// reloadMu serializes reloadWaybar() and auto-restarts so concurrent
// triggers (e.g. two debounce timers firing back-to-back, see GWS-0005; or a
// crash racing a config edit) can't stop/start waybar independently and
// clobber each other's freshly-spawned process.
var reloadMu sync.Mutex

// waybarProc is the sidecar's handle on the waybar process it started (see
// GWS-0027). Liveness is an in-memory channel check, not a pgrep guess: there
// is no window where a dead child is misreported as running, or vice versa.
// The child is always Wait()-ed so it never becomes a zombie.
type waybarProc struct {
	mu         sync.Mutex
	cmd        *exec.Cmd
	done       chan struct{}
	generation int
	expectExit bool
}

// start launches a fresh waybar in its own session, shielding it from
// terminal signals sent to the sidecar's group (the sidecar still stops it on
// shutdown). A background goroutine reaps it via Wait() and, if it exits
// without stop() having been called first, schedules an auto-restart.
func (p *waybarProc) start() error {
	p.mu.Lock()
	cmd := exec.Command("waybar")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return err
	}

	p.generation++
	gen := p.generation
	done := make(chan struct{})
	p.cmd = cmd
	p.done = done
	p.expectExit = false
	p.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		close(done)
		p.onExit(gen)
	}()
	return nil
}

// alive reports whether the currently tracked process is still running.
func (p *waybarProc) alive() bool {
	p.mu.Lock()
	started := p.cmd != nil
	done := p.done
	p.mu.Unlock()
	if !started {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

// pid returns the current child's PID, or 0 if none is tracked.
func (p *waybarProc) pid() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// stop terminates the tracked process (its whole process group, since Setsid
// makes pid == pgid) and waits for it to exit, escalating to SIGKILL if it
// doesn't within stopGraceDelay. Idempotent: a no-op if nothing is tracked.
func (p *waybarProc) stop() {
	p.mu.Lock()
	pid := 0
	done := p.done
	if p.cmd != nil && p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
	}
	p.expectExit = true
	p.mu.Unlock()

	if pid == 0 {
		return
	}

	_ = killFn(-pid, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(stopGraceDelay):
		_ = killFn(-pid, syscall.SIGKILL)
		<-done
	}
}

// onExit runs in the Wait() goroutine after a process exits. If a newer
// start() has since occurred, or stop() marked this exit as expected, the
// caller that triggered it is already handling what comes next and onExit
// does nothing. Otherwise the process died on its own and a restart is
// scheduled, rate-limited.
func (p *waybarProc) onExit(generation int) {
	p.mu.Lock()
	current := p.generation == generation
	expected := p.expectExit
	p.mu.Unlock()

	if !current || expected {
		return
	}
	p.scheduleAutoRestart()
}

var (
	autoRestartMu    sync.Mutex
	autoRestartTimes []time.Time
)

// scheduleAutoRestart restarts a waybar that exited unexpectedly, unless it
// has crashed more than autoRestartMax times within autoRestartWindow.
func (p *waybarProc) scheduleAutoRestart() {
	autoRestartMu.Lock()
	now := time.Now()
	cutoff := now.Add(-autoRestartWindow)
	kept := autoRestartTimes[:0]
	for _, t := range autoRestartTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	autoRestartTimes = kept
	tooMany := len(autoRestartTimes) > autoRestartMax
	autoRestartMu.Unlock()

	if tooMany {
		logger.Error("waybar crashed repeatedly, giving up on auto-restart", "restarts", len(autoRestartTimes))
		return
	}

	logger.Warn("waybar exited unexpectedly, restarting")
	reloadMu.Lock()
	defer reloadMu.Unlock()
	if p.alive() {
		logger.Debug("waybar already running, skipping auto-restart")
		return
	}
	if err := p.start(); err != nil {
		logger.Error("failed to restart waybar after crash", "err", err)
	}
}

// waybar is the sidecar's single supervised waybar instance.
var waybar waybarProc

// reloadWaybar asks waybar to reload in place via SIGUSR2 if it's running,
// falling back to a full stop+start if that fails or waybar isn't running.
func reloadWaybar() {
	reloadMu.Lock()
	defer reloadMu.Unlock()

	if !waybar.alive() {
		logger.Info("waybar not running, performing cold start")
	} else if err := killFn(waybar.pid(), syscall.SIGUSR2); err != nil {
		logger.Warn("USR2 failed, falling back to cold start", "err", err)
	} else {
		logger.Info("waybar reloaded via USR2", "pid", waybar.pid())
		time.Sleep(usr2SettleDelay)
		if waybar.alive() {
			return
		}
		logger.Warn("waybar died after reload, falling back to cold start")
	}

	waybar.stop()

	if err := waybar.start(); err != nil {
		logger.Error("failed to start waybar", "err", err)
		return
	}

	time.Sleep(coldStartSettleDelay)
	if waybar.alive() {
		logger.Info("waybar started", "pid", waybar.pid())
		return
	}
	logger.Error("waybar failed to start")
}

// sweepStrayWaybars terminates every waybar process found at startup: the
// sidecar owns none yet, so all of them are strays from a previous session.
func sweepStrayWaybars() {
	terminatePIDs(waybarPIDsFn())
}

// waybarPIDs returns all PIDs of valid waybar processes (non-zombie, correct exe).
func waybarPIDs() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err == nil && isRealProc(pid, "/waybar") {
			pids = append(pids, pid)
		}
	}
	return pids
}
