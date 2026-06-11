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

	cfg := loadConfig()

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
