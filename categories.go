package main

import "sort"

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
