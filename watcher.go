package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

func watch(trigger func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer w.Close()

	watchPaths := append([]string{gwsDir, waybarDir}, appDirs()...)
	pending := map[string]bool{}
	for _, p := range watchPaths {
		if _, err := os.Stat(p); err != nil {
			pending[p] = true
			// Non-recursive: only the ancestor's own children matter.
			if err := w.Add(nearestExisting(p)); err != nil {
				logger.Debug("failed to watch ancestor of missing path", "path", p, "err", err)
			}
			continue
		}
		addWatch(w, p)
	}

	targets := symlinkTargets(gwsDir, waybarDir)
	parents := make(map[string]bool)
	for target := range targets {
		parents[filepath.Dir(target)] = true
	}
	for parent := range parents {
		if err := w.Add(parent); err != nil {
			logger.Warn("failed to watch symlink target dir", "path", parent, "err", err)
		} else {
			logger.Debug("watching symlink target dir", "path", parent)
		}
	}

	for {
		select {
		case e := <-w.Events:
			if !isRelevantOp(e.Op) {
				continue
			}
			if handleCreate(w, e, watchPaths, pending) {
				logger.Info("app directory appeared", "dir", e.Name)
				trigger()
			}
			if !acceptEvent(e.Name, targets) {
				continue
			}

			logger.Info("change detected", "file", e.Name, "op", e.Op.String())
			trigger()

		case err := <-w.Errors:
			logger.Warn("watcher error", "err", err)
		}
	}
}

// debounce returns a function that runs f once, d after its last call.
func debounce(d time.Duration, f func()) func() {
	var (
		mu    sync.Mutex
		timer *time.Timer
	)
	return func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(d, f)
	}
}

// reload loads the latest config, regenerates outputs, and returns the config
// now in effect. If loading fails (e.g. invalid YAML), the previous cfg is
// kept and nothing is regenerated instead of exiting (see GWS-0017).
func reload(cfg Config) Config {
	if !noGenerate {
		newCfg, err := loadConfig()
		if err != nil {
			logger.Error("reload failed, keeping previous config", "err", err)
			return cfg
		}
		cfg = newCfg
	}
	generate(cfg)
	jobs.restart(cfg.Jobs)
	return cfg
}

// nearestExisting returns p, or its closest existing ancestor.
func nearestExisting(p string) string {
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

func under(name, root string) bool {
	return name == root || strings.HasPrefix(name, root+string(filepath.Separator))
}

// handleCreate starts watching a directory created after startup: one under
// a watched root, the root itself once it appears, or an ancestor of a still
// missing root (so the next level's creation is seen too). It reports whether
// the directory is an app dir (or under one), so the caller can reload and
// list apps already inside it.
func handleCreate(w *fsnotify.Watcher, e fsnotify.Event, roots []string, pending map[string]bool) bool {
	if e.Op&fsnotify.Create == 0 {
		return false
	}
	if info, err := os.Stat(e.Name); err != nil || !info.IsDir() {
		return false
	}

	watched := false
	for root := range pending {
		switch {
		case root == e.Name:
			delete(pending, root)
			watched = true
		case under(root, e.Name):
			if err := w.Add(e.Name); err != nil {
				logger.Warn("failed to watch dir", "path", e.Name, "err", err)
			}
		}
	}
	for _, root := range roots {
		if under(e.Name, root) {
			watched = true
		}
	}
	if !watched {
		return false
	}
	addWatch(w, e.Name)

	for _, dir := range appDirs() {
		if under(e.Name, dir) {
			return true
		}
	}
	return false
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

	for _, dir := range collectWatchDirs(p) {
		if err := w.Add(dir); err != nil {
			logger.Warn("failed to watch dir", "path", dir, "err", err)
		} else {
			logger.Debug("watching dir", "path", dir)
		}
	}
}

// collectWatchDirs recursively lists root and every subdirectory beneath it,
// following directory symlinks (unlike filepath.Walk). Each entry is returned
// under its logical (pre-symlink) path rooted at root, so callers that
// classify events by string-prefix against root (see isWatchedFile) keep
// working even when part of the path is a symlink. A symlink that resolves to
// a directory already visited (e.g. pointing back at an ancestor) is skipped
// to avoid infinite recursion.
func collectWatchDirs(root string) []string {
	visited := make(map[string]bool)
	var dirs []string

	var walk func(logical string)
	walk = func(logical string) {
		real, err := filepath.EvalSymlinks(logical)
		if err != nil || visited[real] {
			return
		}
		visited[real] = true
		dirs = append(dirs, logical)

		entries, err := os.ReadDir(real)
		if err != nil {
			return
		}
		for _, entry := range entries {
			child := filepath.Join(logical, entry.Name())
			if info, err := os.Stat(child); err == nil && info.IsDir() {
				walk(child)
			}
		}
	}
	walk(root)

	return dirs
}

func isRelevantOp(op fsnotify.Op) bool {
	return op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
}

// acceptEvent decides whether an inotify event name should trigger a reload.
// targets is the set of resolved symlink target paths from symlinkTargets.
func acceptEvent(name string, targets map[string]bool) bool {
	if targets[name] {
		return true
	}
	if name == outJSON || strings.HasPrefix(name, menuDir+string(filepath.Separator)) {
		return false
	}
	return isWatchedFile(name)
}

func isWatchedFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if strings.HasPrefix(name, gwsDir+string(filepath.Separator)) {
		return ext == ".yaml" || ext == ".yml"
	}
	for _, dir := range appDirs() {
		if strings.HasPrefix(name, dir+string(filepath.Separator)) {
			return ext == ".desktop"
		}
	}
	if strings.HasPrefix(name, waybarDir+string(filepath.Separator)) {
		return ext == ".json" || ext == ".jsonc" || ext == ".css"
	}
	return false
}

// symlinkTargets scans the given directories (non-recursively) for whitelisted
// files that are symlinks, and returns the set of their resolved real paths.
// This lets watch() add a watch on each target's parent directory so that
// edits to the symlink's target content (which may live outside the watched
// directories) are detected.
func symlinkTargets(dirs ...string) map[string]bool {
	targets := make(map[string]bool)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if !isWatchedFile(path) {
				continue
			}
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				targets[resolved] = true
			}
		}
	}
	return targets
}
