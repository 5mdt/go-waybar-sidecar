package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var pidDir = filepath.Join(os.Getenv("HOME"), ".local/state/waybar")

func ensureSingleInstance() {
	path := filepath.Join(pidDir, "waybar-sidecar.pid")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid == 0 || pid == os.Getpid() {
		return
	}
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return
	}
	if !strings.HasSuffix(exe, "/go-waybar-sidecar") {
		return
	}

	logger.Info("replacing running instance", "pid", pid)
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		logger.Warn("failed to signal previous instance", "pid", pid, "err", err)
		return
	}

	for i := 0; i < 50; i++ {
		if syscall.Kill(pid, 0) != nil {
			logger.Debug("previous instance exited", "pid", pid)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	logger.Warn("previous instance did not exit in time", "pid", pid)
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
