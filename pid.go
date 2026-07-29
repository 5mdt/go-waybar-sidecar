package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var pidDir = filepath.Join(os.Getenv("HOME"), ".local/state/waybar")

// sidecarExeSuffix identifies this binary via /proc/<pid>/exe. Not pgrep-based:
// "go-waybar-sidecar" is 17 characters, and Linux truncates comm to 15, so
// `pgrep -x` can never match it; `pgrep -f` matches unrelated processes whose
// command line merely mentions the name.
const sidecarExeSuffix = "/go-waybar-sidecar"

// Tunables for terminateSidecars/ensureSingleInstance; vars so tests can
// shrink them (mirrors usr2SettleDelay etc. in waybar.go).
var (
	sidecarStopGrace = 5 * time.Second
	sidecarStopPoll  = 100 * time.Millisecond
)

// sidecarPIDsFn is an indirection over sidecarPIDs so tests can stub it.
var sidecarPIDsFn = sidecarPIDs

// instanceLock holds the takeover flock for the process's lifetime. Kept in
// a package var so the fd is never closed or garbage collected; the kernel
// releases it automatically on exit, including a hard kill.
var instanceLock *os.File

// isRealProc reports whether pid is a live (non-zombie) process whose
// executable path ends in exeSuffix.
func isRealProc(pid int, exeSuffix string) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			if strings.Contains(line, "Z") {
				return false
			}
			break
		}
	}
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return false
	}
	return strings.HasSuffix(exe, exeSuffix)
}

// sidecarPIDs scans /proc for other go-waybar-sidecar processes, excluding
// this one.
func sidecarPIDs() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	self := os.Getpid()
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		if isRealProc(pid, sidecarExeSuffix) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// terminateSidecars SIGTERMs pids, waits up to sidecarStopGrace for them to
// exit, then SIGKILLs any survivor.
func terminateSidecars(pids []int) {
	if len(pids) == 0 {
		return
	}

	for _, pid := range pids {
		logger.Info("replacing running instance", "pid", pid)
		if err := killFn(pid, syscall.SIGTERM); err != nil {
			logger.Warn("failed to signal previous instance", "pid", pid, "err", err)
		}
	}

	alive := func() []int {
		var left []int
		for _, pid := range pids {
			if killFn(pid, 0) == nil {
				left = append(left, pid)
			}
		}
		return left
	}

	deadline := time.Now().Add(sidecarStopGrace)
	survivors := alive()
	for len(survivors) > 0 && time.Now().Before(deadline) {
		time.Sleep(sidecarStopPoll)
		survivors = alive()
	}

	for _, pid := range survivors {
		logger.Warn("previous instance did not exit in time, sending SIGKILL", "pid", pid)
		if err := killFn(pid, syscall.SIGKILL); err != nil {
			logger.Warn("failed to SIGKILL previous instance", "pid", pid, "err", err)
		}
	}
}

// acquireInstanceLock takes a non-blocking exclusive flock on
// <pidDir>/waybar-sidecar.lock. locked=false, err=nil means another process
// currently holds it - not a failure.
func acquireInstanceLock() (locked bool, err error) {
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return false, err
	}

	f, err := os.OpenFile(filepath.Join(pidDir, "waybar-sidecar.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}

	instanceLock = f
	return true, nil
}

// ensureSingleInstance guarantees this process is the only go-waybar-sidecar
// running by the time it returns: it terminates every other instance it can
// find via /proc, then takes an exclusive lock so a concurrent launch can't
// slip past the same check. The newest instance always wins the takeover.
func ensureSingleInstance() {
	terminateSidecars(sidecarPIDsFn())

	deadline := time.Now().Add(sidecarStopGrace)
	for {
		locked, err := acquireInstanceLock()
		if err != nil {
			logger.Warn("failed to acquire instance lock", "err", err)
			return
		}
		if locked {
			return
		}
		if !time.Now().Before(deadline) {
			logger.Warn("instance lock still held after timeout, proceeding unlocked")
			return
		}
		time.Sleep(sidecarStopPoll)
	}
}

func savePID(name string, pid int) {
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		logger.Error("failed to create pid dir", "path", pidDir, "err", err)
		return
	}
	path := filepath.Join(pidDir, name+".pid")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		logger.Error("failed to write pid file", "path", path, "err", err)
		return
	}
	logger.Debug("pid saved", "name", name, "pid", pid, "path", path)
}
