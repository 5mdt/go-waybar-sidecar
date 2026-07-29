package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// appDirs returns the XDG application directories in precedence order:
// $XDG_DATA_HOME (default ~/.local/share), then each of $XDG_DATA_DIRS
// (default /usr/local/share:/usr/share), each with /applications appended.
func appDirs() []string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(os.Getenv("HOME"), ".local/share")
	}

	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}

	dirs := []string{filepath.Join(dataHome, "applications")}
	for _, d := range strings.Split(dataDirs, ":") {
		if d == "" {
			continue
		}
		dirs = append(dirs, filepath.Join(d, "applications"))
	}
	return dirs
}

// desktopEntry is the subset of a [Desktop Entry] group needed for a menu item.
type desktopEntry struct {
	name string
	exec string
}

// parseDesktopEntry reads the [Desktop Entry] group from b. It returns
// ok=false when the entry should be skipped: NoDisplay=true, Hidden=true,
// Type is not Application, or Name/Exec is missing.
func parseDesktopEntry(b []byte) (desktopEntry, bool) {
	var (
		e         desktopEntry
		typ       string
		noDisplay bool
		hidden    bool
		inEntry   bool
	)

	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "Name":
			e.name = value
		case "Exec":
			e.exec = value
		case "Type":
			typ = value
		case "NoDisplay":
			noDisplay = value == "true"
		case "Hidden":
			hidden = value == "true"
		}
	}

	if noDisplay || hidden || typ != "Application" || e.name == "" || e.exec == "" {
		return desktopEntry{}, false
	}
	return e, true
}

// fieldCodes are Exec field codes stripped from a command line before it is
// used as a menu action (they are only meaningful when the launcher passes
// filenames/URLs, which the sidecar never does).
var fieldCodes = []string{"%f", "%F", "%u", "%U", "%d", "%D", "%n", "%N", "%i", "%c", "%k", "%v", "%m"}

// stripFieldCodes removes Exec field codes and unescapes %% to %.
func stripFieldCodes(exec string) string {
	for _, code := range fieldCodes {
		exec = strings.ReplaceAll(exec, code, "")
	}
	exec = strings.ReplaceAll(exec, "%%", "%")
	return strings.Join(strings.Fields(exec), " ")
}

// scanApplications walks appDirs() and returns MenuItems sorted by Name.
// Entries are deduped by desktop file id (path relative to its app dir, with
// "/" replaced by "-"); the first directory in precedence order wins.
// It is a package-level var so tests can stub it.
var scanApplications = func() []MenuItem {
	seen := map[string]bool{}
	var items []MenuItem

	for _, dir := range appDirs() {
		entries, err := desktopFilesIn(dir)
		if err != nil {
			continue
		}
		for id, b := range entries {
			if seen[id] {
				continue
			}
			seen[id] = true

			entry, ok := parseDesktopEntry(b)
			if !ok {
				continue
			}
			items = append(items, MenuItem{
				Name:   entry.name,
				Action: stripFieldCodes(entry.exec),
			})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

// desktopFilesIn recursively collects *.desktop files under dir, keyed by
// their desktop file id (path relative to dir, "/" replaced by "-").
func desktopFilesIn(dir string) (map[string][]byte, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}

	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".desktop" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			logger.Debug("failed to read desktop file", "path", path, "err", err)
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		id := strings.ReplaceAll(rel, string(filepath.Separator), "-")
		files[id] = b
		return nil
	})
	return files, err
}
