package main

import (
	"strings"
	"testing"
)

// tree renders submenus as "Label:[child,child]" for compact comparison.
func tree(items []MenuItem) []string {
	var out []string
	for _, it := range items {
		var names []string
		for _, c := range it.Children {
			names = append(names, c.Name)
		}
		out = append(out, it.Name+":["+strings.Join(names, ",")+"]")
	}
	return out
}

func TestCategorizeApps(t *testing.T) {
	apps := []MenuItem{
		{Name: "Browser", Categories: []string{"Network", "WebBrowser", "Utility"}},
		{Name: "Editor", Categories: []string{"Utility", "Development"}},
		{Name: "Player", Categories: []string{"AudioVideo", "Audio"}},
		{Name: "Solitaire", Categories: []string{"Game"}},
		{Name: "Widget", Categories: []string{"GTK", "Qt"}},
		{Name: "Zip"},
	}

	allWant := []string{
		"Audio & Video:[Player]",
		"Development:[Editor]",
		"Games:[Solitaire]",
		"Network:[Browser]",
		"Utility:[Browser,Editor]",
		"Other:[Widget,Zip]",
	}

	tests := []struct {
		name string
		mode string
		want []string
	}{
		{"first", "first", []string{
			"Audio & Video:[Player]",
			"Games:[Solitaire]",
			"Network:[Browser]",
			"Utility:[Editor]",
			"Other:[Widget,Zip]",
		}},
		{"all", "all", allWant},
		{"default is all", "", allWant},
		{"invalid falls back to all", "bogus", allWant},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tree(categorizeApps(apps, tt.mode))
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("categorizeApps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCategorizeApps_NoOther(t *testing.T) {
	got := tree(categorizeApps([]MenuItem{{Name: "Editor", Categories: []string{"Development"}}}, "first"))
	if len(got) != 1 || got[0] != "Development:[Editor]" {
		t.Errorf("categorizeApps() = %v, want only Development", got)
	}
}
func TestCategoryIcons(t *testing.T) {
	labels := []string{otherCategory}
	for _, l := range mainCategories {
		labels = append(labels, l)
	}
	for _, l := range labels {
		if categoryIcons[l] == "" {
			t.Errorf("categoryIcons[%q] missing", l)
		}
	}
}

func TestCategorizeAppsIcons(t *testing.T) {
	got := categorizeApps([]MenuItem{
		{Name: "Editor", Categories: []string{"Development"}},
		{Name: "Misc"},
	}, "all")
	want := map[string]string{"Development": "applications-development", "Other": "applications-other"}
	if len(got) != len(want) {
		t.Fatalf("got %d submenus, want %d", len(got), len(want))
	}
	for _, m := range got {
		if m.Icon != want[m.Name] {
			t.Errorf("%s icon = %q, want %q", m.Name, m.Icon, want[m.Name])
		}
	}
}
