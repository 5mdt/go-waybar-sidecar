package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMenuFileName(t *testing.T) {
	cases := map[string]string{
		"custom/Start": "custom-start.xml",
		"custom/Power": "custom-power.xml",
		"clock":        "clock.xml",
	}
	for in, want := range cases {
		if got := menuFileName(in); got != want {
			t.Errorf("menuFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildXMLAndWriteJSON(t *testing.T) {
	dir := t.TempDir()

	origMenuDir, origOutJSON := menuDir, outJSON
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() { menuDir, outJSON = origMenuDir, origOutJSON }()

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Waybar: map[string]any{
			"custom/Start": map[string]any{
				"format":          "Start",
				"menu_definition": "placeholder",
			},
			"custom/Power": map[string]any{
				"format":          "Power",
				"menu_definition": "placeholder",
			},
			"clock": map[string]any{
				"format": "{:%H:%M}",
			},
		},
		Menus: map[string][]MenuItem{
			"custom/Start": {{Name: "Terminal", Action: "foot"}},
			"custom/Power": {{Name: "Shutdown", Action: "systemctl poweroff"}},
		},
	}

	menus := buildXML(cfg)
	if len(menus) != 2 {
		t.Fatalf("expected 2 menus, got %d", len(menus))
	}
	for name, res := range menus {
		if _, err := os.Stat(res.path); err != nil {
			t.Errorf("menu %q: xml file not written: %v", name, err)
		}
	}
	if menus["custom/Start"].path != filepath.Join(menuDir, "custom-start.xml") {
		t.Errorf("custom/Start path = %q", menus["custom/Start"].path)
	}

	writeJSON(cfg.Waybar, menus)

	data, err := os.ReadFile(outJSON)
	if err != nil {
		t.Fatalf("reading output json: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	for _, name := range []string{"custom/Start", "custom/Power"} {
		mod, ok := out[name].(map[string]any)
		if !ok {
			t.Fatalf("missing module %q in output", name)
		}
		if _, ok := mod["menu_definition"]; ok {
			t.Errorf("%s: menu_definition should be removed", name)
		}
		if mod["menu"] != "on-click" {
			t.Errorf("%s: menu = %v, want on-click", name, mod["menu"])
		}
		if mod["menu-file"] != menus[name].path {
			t.Errorf("%s: menu-file = %v, want %v", name, mod["menu-file"], menus[name].path)
		}
		if _, ok := mod["menu-actions"]; !ok {
			t.Errorf("%s: menu-actions not set", name)
		}
	}

	clock, ok := out["clock"].(map[string]any)
	if !ok {
		t.Fatal("missing clock module in output")
	}
	if _, ok := clock["menu-file"]; ok {
		t.Error("clock should not have menu-file")
	}
}

func TestGenerateNoGenerate(t *testing.T) {
	dir := t.TempDir()

	origMenuDir, origOutJSON, origNoGenerate, origDoReload := menuDir, outJSON, noGenerate, doReload
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() {
		menuDir, outJSON, noGenerate, doReload = origMenuDir, origOutJSON, origNoGenerate, origDoReload
	}()

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		t.Fatal(err)
	}

	reloadCount := 0
	doReload = func() { reloadCount++ }

	noGenerate = true
	generate(Config{})

	if reloadCount != 1 {
		t.Errorf("reload calls = %d, want 1", reloadCount)
	}
	if _, err := os.Stat(outJSON); !os.IsNotExist(err) {
		t.Errorf("expected %s to not exist, err = %v", outJSON, err)
	}
	entries, err := os.ReadDir(menuDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected menuDir to be empty, found %d entries", len(entries))
	}

	noGenerate = false
	cfg := Config{
		Waybar: map[string]any{
			"custom/Start": map[string]any{
				"format":          "Start",
				"menu_definition": "placeholder",
			},
		},
		Menus: map[string][]MenuItem{
			"custom/Start": {{Name: "Terminal", Action: "foot"}},
		},
	}
	generate(cfg)

	if reloadCount != 2 {
		t.Errorf("reload calls = %d, want 2", reloadCount)
	}
	if _, err := os.Stat(outJSON); err != nil {
		t.Errorf("expected %s to exist: %v", outJSON, err)
	}
}
