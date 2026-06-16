package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsWatchedFile(t *testing.T) {
	dir := t.TempDir()
	origWaybarDir, origGwsDir := waybarDir, gwsDir
	waybarDir = dir
	gwsDir = filepath.Join(dir, "go-waybar-sidecar")
	defer func() {
		waybarDir, gwsDir = origWaybarDir, origGwsDir
	}()

	waybarCases := map[string]bool{
		filepath.Join(dir, "config.jsonc"):     true,
		filepath.Join(dir, "style.css"):        true,
		filepath.Join(dir, "foo.json"):         true,
		filepath.Join(dir, "config.yaml"):      false,
		filepath.Join(dir, "config.jsonc.bak"): false,
		filepath.Join(dir, "4913"):             false,
		filepath.Join(dir, "style.css~"):       false,
	}
	for name, want := range waybarCases {
		if got := isWatchedFile(name); got != want {
			t.Errorf("isWatchedFile(%q) = %v, want %v", name, got, want)
		}
	}

	gwsCases := map[string]bool{
		filepath.Join(gwsDir, "config.yaml"):       true,
		filepath.Join(gwsDir, "foo.yml"):           true,
		filepath.Join(gwsDir, "config.json"):       false,
		filepath.Join(gwsDir, ".config.yaml.swp"):  false,
		filepath.Join(gwsDir, "config.yaml~"):      false,
	}
	for name, want := range gwsCases {
		if got := isWatchedFile(name); got != want {
			t.Errorf("isWatchedFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestReloadKeepsPreviousConfigOnError(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(badFile, []byte("waybar: [this is not a map"), 0o644); err != nil {
		t.Fatal(err)
	}

	origConfigPath, origNoGenerate, origDoReload := configPath, noGenerate, doReload
	origMenuDir, origOutJSON := menuDir, outJSON
	configPath = badFile
	noGenerate = false
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() {
		configPath, noGenerate, doReload = origConfigPath, origNoGenerate, origDoReload
		menuDir, outJSON = origMenuDir, origOutJSON
	}()

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		t.Fatal(err)
	}

	reloadCount := 0
	doReload = func() { reloadCount++ }

	cfg := Config{Waybar: map[string]any{"layer": "top"}}
	reload(&cfg)

	if _, ok := cfg.Waybar["layer"]; !ok {
		t.Error("expected previous config to be kept after reload error")
	}
	if reloadCount != 0 {
		t.Errorf("reload calls = %d, want 0 (no regeneration on load error)", reloadCount)
	}
}
