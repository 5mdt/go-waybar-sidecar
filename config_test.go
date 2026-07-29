package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

func TestLoadConfigMultipleMenus(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
waybar:
  layer: top
  modules-left: [custom/Start, custom/Power]
  custom/Start:
    format: "Start"
    menu_definition:
      - name: Apps
        children:
          - name: Terminal
            action: foot
  custom/Power:
    format: "Power"
    menu_definition:
      - name: Shutdown
        action: systemctl poweroff
  clock:
    format: "{:%H:%M}"
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	origConfigPath := configPath
	configPath = cfgFile
	defer func() { configPath = origConfigPath }()

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if len(cfg.Menus) != 2 {
		t.Fatalf("expected 2 menus, got %d: %v", len(cfg.Menus), cfg.Menus)
	}

	start, ok := cfg.Menus["custom/Start"]
	if !ok {
		t.Fatal("expected custom/Start menu")
	}
	if len(start) != 1 || start[0].Name != "Apps" || len(start[0].Children) != 1 || start[0].Children[0].Name != "Terminal" || start[0].Children[0].Action != "foot" {
		t.Fatalf("unexpected custom/Start menu: %+v", start)
	}

	power, ok := cfg.Menus["custom/Power"]
	if !ok {
		t.Fatal("expected custom/Power menu")
	}
	if len(power) != 1 || power[0].Name != "Shutdown" || power[0].Action != "systemctl poweroff" {
		t.Fatalf("unexpected custom/Power menu: %+v", power)
	}

	if _, ok := cfg.Menus["clock"]; ok {
		t.Fatal("clock should not have a menu (no menu_definition)")
	}

	if _, ok := cfg.Waybar["layer"]; !ok {
		t.Fatal("expected layer key in waybar config")
	}
}

func TestParseConfigInvalid(t *testing.T) {
	_, err := parseConfig([]byte("waybar: [this is not a map"))
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestParseConfigDefault(t *testing.T) {
	if _, err := parseConfig(defaultConfig); err != nil {
		t.Fatalf("parseConfig(defaultConfig) error = %v", err)
	}
}

func TestParseConfigJobs(t *testing.T) {
	yaml := `
waybar:
  layer: top
jobs:
  - name: sync
    every: 5m
    command: mbsync -a
  - name: weather
    schedule: "*/15 * * * *"
    command: "curl wttr.in"
    output: ~/.cache/waybar/weather
`
	cfg, err := parseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if len(cfg.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(cfg.Jobs))
	}

	j0 := cfg.Jobs[0]
	if j0.Name != "sync" {
		t.Errorf("job[0].Name = %q, want %q", j0.Name, "sync")
	}
	if j0.Every != "5m" {
		t.Errorf("job[0].Every = %q, want %q", j0.Every, "5m")
	}
	if j0.Command != "mbsync -a" {
		t.Errorf("job[0].Command = %q, want %q", j0.Command, "mbsync -a")
	}
	if j0.Output != "" {
		t.Errorf("job[0].Output = %q, want empty", j0.Output)
	}

	j1 := cfg.Jobs[1]
	if j1.Name != "weather" {
		t.Errorf("job[1].Name = %q, want %q", j1.Name, "weather")
	}
	if j1.Schedule != "*/15 * * * *" {
		t.Errorf("job[1].Schedule = %q, want %q", j1.Schedule, "*/15 * * * *")
	}
	if j1.Output != "~/.cache/waybar/weather" {
		t.Errorf("job[1].Output = %q, want %q", j1.Output, "~/.cache/waybar/weather")
	}
}

func TestExpandHome(t *testing.T) {
	home := os.Getenv("HOME")
	tests := map[string]string{
		"~/.config/foo": home + "/.config/foo",
		"~/":            home + "/",
		"~":             home,
		"/abs/path":     "/abs/path",
		"relative/path": "relative/path",
		"":              "",
	}
	for input, want := range tests {
		got := expandHome(input)
		if got != want {
			t.Errorf("expandHome(%q) = %q, want %q", input, got, want)
		}
	}
}

func stubScanApplications(t *testing.T, items []MenuItem) {
	t.Helper()
	orig := scanApplications
	scanApplications = func() []MenuItem { return items }
	t.Cleanup(func() { scanApplications = orig })
}

func TestExpandIncludes_Applications(t *testing.T) {
	stubScanApplications(t, []MenuItem{
		{Name: "Alpha", Action: "alpha"},
		{Name: "Beta", Action: "beta"},
	})

	items := []MenuItem{
		{Name: "Terminal", Action: "foot"},
		{Include: "applications:plain"},
		{Name: "Browser", Action: "firefox"},
	}
	got := expandIncludes(items)

	want := []string{"Terminal", "Alpha", "Beta", "Browser"}
	if len(got) != len(want) {
		t.Fatalf("expandIncludes() = %d items, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestExpandIncludes_TopLevel(t *testing.T) {
	stubScanApplications(t, []MenuItem{{Name: "Alpha", Action: "alpha"}})

	got := expandIncludes([]MenuItem{{Include: "applications:plain"}})
	if len(got) != 1 || got[0].Name != "Alpha" {
		t.Fatalf("expandIncludes() = %+v, want [Alpha]", got)
	}
}

func TestExpandIncludes_Nested(t *testing.T) {
	stubScanApplications(t, []MenuItem{{Name: "Alpha", Action: "alpha"}})

	items := []MenuItem{
		{
			Name: "Apps",
			Children: []MenuItem{
				{Include: "applications:plain"},
			},
		},
	}
	got := expandIncludes(items)

	if len(got) != 1 || got[0].Name != "Apps" {
		t.Fatalf("expandIncludes() = %+v, want single Apps submenu", got)
	}
	children := got[0].Children
	if len(children) != 1 || children[0].Name != "Alpha" {
		t.Fatalf("expandIncludes() children = %+v, want [Alpha]", children)
	}
}

func TestExpandIncludes_UnknownAlias(t *testing.T) {
	items := []MenuItem{
		{Name: "Terminal", Action: "foot"},
		{Include: "applications:bogus"},
		{Name: "Browser", Action: "firefox"},
	}
	got := expandIncludes(items)

	want := []string{"Terminal", "Browser"}
	if len(got) != len(want) {
		t.Fatalf("expandIncludes() = %+v, want %v (unknown alias dropped)", got, want)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("got[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestExpandIncludes_NoIncludes(t *testing.T) {
	items := []MenuItem{
		{Name: "Terminal", Action: "foot"},
		{Name: "Apps", Children: []MenuItem{{Name: "Browser", Action: "firefox"}}},
	}
	got := expandIncludes(items)

	if len(got) != 2 || got[0].Name != "Terminal" || got[1].Name != "Apps" {
		t.Fatalf("expandIncludes() = %+v, want unchanged passthrough", got)
	}
	if len(got[1].Children) != 1 || got[1].Children[0].Name != "Browser" {
		t.Fatalf("expandIncludes() children = %+v, want unchanged", got[1].Children)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	origConfigPath := configPath
	configPath = filepath.Join(t.TempDir(), "does-not-exist.yaml")
	defer func() { configPath = origConfigPath }()

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}
