package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func reloadWaybar() {
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
