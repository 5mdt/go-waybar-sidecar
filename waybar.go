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

func reloadWaybar() {
	if pid := waybarPID(); pid != 0 {
		if err := syscall.Kill(pid, syscall.SIGUSR2); err == nil {
			logger.Info("waybar reloaded via USR2", "pid", pid)
			savePID("waybar", pid)
			time.Sleep(500 * time.Millisecond)
			if waybarPID() != 0 {
				return
			}
			logger.Warn("waybar died after reload, falling back to cold start")
		}
	}

	logger.Warn("waybar not running, performing cold start")
	exec.Command("pkill", "-x", "waybar").Run()
	time.Sleep(200 * time.Millisecond)

	cmd := exec.Command("waybar")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logger.Error("failed to start waybar", "err", err)
		return
	}

	for i := 0; i < 4; i++ {
		time.Sleep(500 * time.Millisecond)
		if alive := waybarPID(); alive != 0 {
			logger.Info("waybar started", "pid", alive)
			savePID("waybar", alive)
			return
		}
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
