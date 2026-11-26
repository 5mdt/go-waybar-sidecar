package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var pidDir = filepath.Join(os.Getenv("HOME"), ".local/state/waybar")

// Tunables for terminatePIDs/ensureSingleInstance; vars so tests can
// shrink them (mirrors usr2SettleDelay etc. in waybar.go).
var (
	sidecarStopGrace = 5 * time.Second
	sidecarStopPoll  = 100 * time.Millisecond
)

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

// terminatePIDs SIGTERMs pids, waits up to sidecarStopGrace for them to
// exit, then SIGKILLs any survivor.
func terminatePIDs(pids []int) {
	if len(pids) == 0 {
		return
	}

	for _, pid := range pids {
		logger.Info("terminating process", "pid", pid)
		if err := killFn(pid, syscall.SIGTERM); err != nil {
			logger.Warn("failed to signal process", "pid", pid, "err", err)
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
		logger.Warn("process did not exit in time, sending SIGKILL", "pid", pid)
		if err := killFn(pid, syscall.SIGKILL); err != nil {
			logger.Warn("failed to SIGKILL process", "pid", pid, "err", err)
		}
	}
}

// acquireInstanceLock takes a non-blocking exclusive flock on
// <pidDir>/waybar-sidecar.lock and records this process's PID in it. When
// another process holds the lock it returns f=nil and that process's PID
// (0 if the file is empty or unreadable) - not an error.
func acquireInstanceLock() (f *os.File, holder int, err error) {
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return nil, 0, err
	}

	f, err = os.OpenFile(filepath.Join(pidDir, "waybar-sidecar.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, 0, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, 0, err
		}
		b, _ := io.ReadAll(f)
		holder, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		return nil, holder, nil
	}

	if err := f.Truncate(0); err == nil {
		_, err = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	}
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, 0, nil
}

// ensureSingleInstance takes the instance lock, terminating the PID recorded
// in it when another instance holds it: the newest instance always wins. The
// holder is re-read every round, so one that had not yet written its PID is
// still caught. After sidecarStopGrace it proceeds unlocked with a warning
// rather than hang.
func ensureSingleInstance() {
	deadline := time.Now().Add(sidecarStopGrace)
	signalled := 0
	for {
		f, holder, err := acquireInstanceLock()
		if err != nil {
			logger.Warn("failed to acquire instance lock", "err", err)
			return
		}
		if f != nil {
			instanceLock = f
			return
		}
		if holder > 0 && holder != signalled {
			signalled = holder
			terminatePIDs([]int{holder})
			continue
		}
		if !time.Now().Before(deadline) {
			logger.Warn("instance lock still held after timeout, proceeding unlocked")
			return
		}
		time.Sleep(sidecarStopPoll)
	}
}
