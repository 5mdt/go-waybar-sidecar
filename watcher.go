package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func watch(cfg *Config) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("failed to create watcher", "err", err)
		os.Exit(1)
	}
	defer w.Close()

	for _, p := range []string{configPath, waybarDir} {
		addWatch(w, p)
	}

	var (
		timer   *time.Timer
		timerMu sync.Mutex
	)

	for {
		select {
		case e := <-w.Events:
			if !isRelevantOp(e.Op) {
				continue
			}
			if e.Name == outXML || e.Name == outJSON {
				continue
			}
			if isIgnoredFile(e.Name) {
				continue
			}

			logger.Info("change detected", "file", e.Name, "op", e.Op.String())

			timerMu.Lock()
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(200*time.Millisecond, func() {
				*cfg = loadConfig()
				generate(*cfg)
			})
			timerMu.Unlock()

		case err := <-w.Errors:
			logger.Warn("watcher error", "err", err)
		}
	}
}

func addWatch(w *fsnotify.Watcher, p string) {
	info, err := os.Stat(p)
	if err != nil {
		logger.Debug("skipping watch path (not found)", "path", p)
		return
	}

	if !info.IsDir() {
		if err := w.Add(p); err != nil {
			logger.Warn("failed to watch file", "path", p, "err", err)
		} else {
			logger.Debug("watching file", "path", p)
		}
		return
	}

	filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if err := w.Add(path); err != nil {
			logger.Warn("failed to watch dir", "path", path, "err", err)
		} else {
			logger.Debug("watching dir", "path", path)
		}
		return nil
	})
}

func isRelevantOp(op fsnotify.Op) bool {
	return op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
}

func isIgnoredFile(name string) bool {
	base := filepath.Base(name)
	// vim swap/backup files
	if strings.HasPrefix(base, ".") && (strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, ".swo") ||
		strings.HasSuffix(base, ".swn")) {
		return true
	}
	// vim backup
	if strings.HasSuffix(base, "~") {
		return true
	}
	// vim write-test file (4913, etc.)
	allDigits := len(base) > 0
	for _, c := range base {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	return allDigits
}
