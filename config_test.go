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

func TestLoadConfigMissingFile(t *testing.T) {
	origConfigPath := configPath
	configPath = filepath.Join(t.TempDir(), "does-not-exist.yaml")
	defer func() { configPath = origConfigPath }()

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
}
