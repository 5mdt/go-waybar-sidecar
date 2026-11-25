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
	name       string
	exec       string
	icon       string
	categories []string
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
		case "Icon":
			e.icon = value
		case "Categories":
			for _, c := range strings.Split(value, ";") {
				if c = strings.TrimSpace(c); c != "" {
					e.categories = append(e.categories, c)
				}
			}
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
				Name:       entry.name,
				Action:     stripFieldCodes(entry.exec),
				Icon:       entry.icon,
				Categories: entry.categories,
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

// mainCategories maps freedesktop main category keys to menu labels.
var mainCategories = map[string]string{
	"AudioVideo":  "Audio & Video",
	"Development": "Development",
	"Education":   "Education",
	"Game":        "Games",
	"Graphics":    "Graphics",
	"Network":     "Network",
	"Office":      "Office",
	"Science":     "Science",
	"Settings":    "Settings",
	"System":      "System",
	"Utility":     "Utility",
}

const otherCategory = "Other"

// categoryIcons maps category labels to freedesktop icon-theme names (GWS-0032).
var categoryIcons = map[string]string{
	"Audio & Video": "applications-multimedia",
	"Development":   "applications-development",
	"Education":     "applications-education",
	"Games":         "applications-games",
	"Graphics":      "applications-graphics",
	"Network":       "applications-internet",
	"Office":        "applications-office",
	"Science":       "applications-science",
	"Settings":      "preferences-desktop",
	"System":        "applications-system",
	"Utility":       "applications-utilities",
	otherCategory:   "applications-other",
}

// categorizeApps groups apps (assumed name-sorted) into one submenu per
// non-empty main category, sorted by label with "Other" last. mode "first"
// places an app in the first main category it lists; anything else ("all",
// "", unknown with a warning) places it in every one. Apps with no main
// category go to "Other". See GWS-0031.
func categorizeApps(apps []MenuItem, mode string) []MenuItem {
	if mode != "" && mode != "first" && mode != "all" {
		logger.Warn("unknown include mode, using all", "mode", mode)
	}

	buckets := map[string][]MenuItem{}
	for _, app := range apps {
		var labels []string
		for _, c := range app.Categories {
			if label, ok := mainCategories[c]; ok {
				labels = append(labels, label)
			}
		}
		switch {
		case len(labels) == 0:
			labels = []string{otherCategory}
		case mode == "first":
			labels = labels[:1]
		}

		app.Categories = nil
		for _, label := range labels {
			buckets[label] = append(buckets[label], app)
		}
	}

	var menus []MenuItem
	for label, children := range buckets {
		menus = append(menus, MenuItem{Name: label, Icon: categoryIcons[label], Children: children})
	}
	sort.Slice(menus, func(i, j int) bool {
		if (menus[i].Name == otherCategory) != (menus[j].Name == otherCategory) {
			return menus[j].Name == otherCategory
		}
		return menus[i].Name < menus[j].Name
	})
	return menus
}
