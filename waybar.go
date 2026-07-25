package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// coldStartPollAttempts and the sleep durations are vars so tests can shorten them.
var (
	coldStartPollAttempts = 4
	coldStartPollSleep    = 500 * time.Millisecond
	coldStartKillSleep    = 200 * time.Millisecond
)

// waybarPIDFn is an indirection over waybarPID so tests can stub it.
var waybarPIDFn = waybarPID

// Indirections for reapDuplicateWaybars so tests can stub them.
var (
	waybarPIDsFn    = waybarPIDs
	procStartTimeFn = procStartTime
	killFn          = syscall.Kill
)

func reapDuplicateWaybars() {
	pids := waybarPIDsFn()
	if len(pids) <= 1 {
		return
	}
	var newest int
	var newestTime uint64
	for _, pid := range pids {
		st, err := procStartTimeFn(pid)
		if err != nil {
			continue
		}
		if st > newestTime {
			newestTime = st
			newest = pid
		}
	}
	for _, pid := range pids {
		if pid == newest {
			continue
		}
		if err := killFn(pid, syscall.SIGTERM); err == nil {
			logger.Info("killed older waybar instance", "pid", pid, "kept", newest)
		}
	}
}

// reloadMu serializes reloadWaybar() so concurrent reloads (e.g. two debounce
// timers firing back-to-back, see GWS-0005) can't pkill/start waybar
// independently and clobber each other's freshly-spawned process.
var reloadMu sync.Mutex

func reloadWaybar() {
	reloadMu.Lock()
	defer reloadMu.Unlock()

	reapDuplicateWaybars()
	if pid := waybarPIDFn(); pid != 0 {
		if err := syscall.Kill(pid, syscall.SIGUSR2); err == nil {
			logger.Info("waybar reloaded via USR2", "pid", pid)
			time.Sleep(500 * time.Millisecond)
			if waybarPIDFn() != 0 {
				return
			}
			logger.Warn("waybar died after reload, falling back to cold start")
		}
	}

	logger.Warn("waybar not running, performing cold start")
	exec.Command("pkill", "-x", "waybar").Run()
	time.Sleep(coldStartKillSleep)

	cmd := exec.Command("waybar")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logger.Error("failed to start waybar", "err", err)
		return
	}

	for i := 0; i < coldStartPollAttempts; i++ {
		time.Sleep(coldStartPollSleep)
		if alive := waybarPIDFn(); alive != 0 {
			logger.Info("waybar started", "pid", alive)
			return
		}
	}
	// Final check: waybar may have started under a different PID than we
	// launched, or pgrep was briefly slow during the poll window.
	if alive := waybarPIDFn(); alive != 0 {
		logger.Info("waybar started", "pid", alive)
		return
	}
	logger.Error("waybar failed to start within 2s")
}

func waybarPID() int {
	out, err := exec.Command("pgrep", "-x", "waybar").Output()
	if err != nil {
		return 0
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	pid, _ := strconv.Atoi(line)
	if pid == 0 || !isRealWaybar(pid) {
		return 0
	}
	return pid
}

func isRealWaybar(pid int) bool {
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
	return strings.HasSuffix(exe, "/waybar")
}

// waybarPIDs returns all PIDs of valid waybar processes (non-zombie, correct exe).
func waybarPIDs() []int {
	out, err := exec.Command("pgrep", "-x", "waybar").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid == 0 {
			continue
		}
		if isRealWaybar(pid) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// procStartTime returns the kernel start time (field 22 of /proc/<pid>/stat)
// as a raw clock-tick counter. Higher means more recently started.
func procStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// The comm field (2nd token) may contain spaces and parentheses.
	// Skip past the last ')' to find the remaining fields reliably.
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return 0, fmt.Errorf("malformed /proc/%d/stat", pid)
	}
	fields := strings.Fields(s[idx+1:])
	// After ')': state(0) ppid(1) pgrp(2) session(3) tty(4) tpgid(5)
	// flags(6) minflt(7) cminflt(8) majflt(9) cmajflt(10) utime(11)
	// stime(12) cutime(13) cstime(14) priority(15) nice(16) threads(17)
	// itrealvalue(18) starttime(19)
	if len(fields) < 20 {
		return 0, fmt.Errorf("short /proc/%d/stat", pid)
	}
	st, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse starttime for pid %d: %w", pid, err)
	}
	return st, nil
}
