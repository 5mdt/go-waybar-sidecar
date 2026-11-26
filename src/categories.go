package main

import "slices"

type category struct{ key, label, icon string }

// mainCategories lists the freedesktop main categories in menu (label) order,
// with their labels and freedesktop icon-theme names (GWS-0032).
var mainCategories = []category{
	{"AudioVideo", "Audio & Video", "applications-multimedia"},
	{"Development", "Development", "applications-development"},
	{"Education", "Education", "applications-education"},
	{"Game", "Games", "applications-games"},
	{"Graphics", "Graphics", "applications-graphics"},
	{"Network", "Network", "applications-internet"},
	{"Office", "Office", "applications-office"},
	{"Science", "Science", "applications-science"},
	{"Settings", "Settings", "preferences-desktop"},
	{"System", "System", "applications-system"},
	{"Utility", "Utility", "applications-utilities"},
}

// otherCategory collects apps with no main category; it is listed last.
var otherCategory = category{"", "Other", "applications-other"}

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
		for _, key := range app.Categories {
			for _, c := range mainCategories {
				if c.key == key {
					labels = append(labels, c.label)
				}
			}
		}
		switch {
		case len(labels) == 0:
			labels = []string{otherCategory.label}
		case mode == "first":
			labels = labels[:1]
		}

		app.Categories = nil
		for _, label := range labels {
			buckets[label] = append(buckets[label], app)
		}
	}

	var menus []MenuItem
	for _, c := range append(slices.Clone(mainCategories), otherCategory) {
		if children := buckets[c.label]; len(children) > 0 {
			menus = append(menus, MenuItem{Name: c.label, Icon: c.icon, Children: children})
		}
	}
	return menus
}
