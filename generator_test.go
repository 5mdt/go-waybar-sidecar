package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
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

func TestBuildXMLCreatesMenuDir(t *testing.T) {
	dir := t.TempDir()

	origMenuDir := menuDir
	menuDir = filepath.Join(dir, "gtk3-menus")
	defer func() { menuDir = origMenuDir }()

	cfg := Config{
		Waybar: map[string]any{},
		Menus:  map[string][]MenuItem{"custom/Start": {{Name: "Terminal", Action: "foot"}}},
	}
	buildXML(cfg)

	if _, err := os.Stat(menuDir); err != nil {
		t.Errorf("expected menuDir to be created, got: %v", err)
	}
}

func TestBuildXMLAndWriteJSON(t *testing.T) {
	dir := t.TempDir()

	origMenuDir, origOutJSON := menuDir, outJSON
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() { menuDir, outJSON = origMenuDir, origOutJSON }()

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

func TestBackupFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.jsonc")
	bak := path + ".bak"

	// no-op when the file does not exist
	if err := backupFile(path); err != nil {
		t.Fatalf("backupFile on missing file: %v", err)
	}
	if _, err := os.Stat(bak); !os.IsNotExist(err) {
		t.Errorf("expected no %s, err = %v", bak, err)
	}

	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := backupFile(path); err != nil {
		t.Fatalf("backupFile: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be renamed away, err = %v", path, err)
	}
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(data) != "old" {
		t.Errorf("backup content = %q, want %q", data, "old")
	}
}

func TestWriteJSONBacksUpPreviousOutput(t *testing.T) {
	dir := t.TempDir()

	origMenuDir, origOutJSON := menuDir, outJSON
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() { menuDir, outJSON = origMenuDir, origOutJSON }()

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeJSON(map[string]any{"clock": map[string]any{"format": "first"}}, nil)
	writeJSON(map[string]any{"clock": map[string]any{"format": "second"}}, nil)

	bakData, err := os.ReadFile(outJSON + ".bak")
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if !strings.Contains(string(bakData), "first") {
		t.Errorf("backup content = %s, want it to contain %q", bakData, "first")
	}

	curData, err := os.ReadFile(outJSON)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if !strings.Contains(string(curData), "second") {
		t.Errorf("output content = %s, want it to contain %q", curData, "second")
	}
}

func TestValidateMenuDocDetectsDuplicateIDs(t *testing.T) {
	makeDoc := func(ids ...string) *etree.Document {
		doc := etree.NewDocument()
		root := doc.CreateElement("interface")
		for _, id := range ids {
			obj := root.CreateElement("object")
			obj.CreateAttr("id", id)
		}
		return doc
	}

	if err := validateMenuDoc(makeDoc("A", "B", "C")); err != nil {
		t.Errorf("unique ids: unexpected error: %v", err)
	}
	if err := validateMenuDoc(makeDoc("A", "B", "A")); err == nil {
		t.Error("duplicate ids: expected error, got nil")
	}
	if err := validateMenuDoc(makeDoc()); err != nil {
		t.Errorf("empty doc: unexpected error: %v", err)
	}
}

func TestBuildXMLSkipsInvalidMenu(t *testing.T) {
	dir := t.TempDir()
	origMenuDir := menuDir
	origValidate := validateMenu
	menuDir = filepath.Join(dir, "gtk3-menus")
	defer func() { menuDir, validateMenu = origMenuDir, origValidate }()

	// Stub: reject menus with exactly 2 <object id> elements
	// (the GtkMenu root "menu" + one leaf item = bad menu shape in this test).
	validateMenu = func(doc *etree.Document) error {
		if len(doc.Root().FindElements("//object[@id]")) == 2 {
			return fmt.Errorf("injected validation error")
		}
		return nil
	}

	cfg := Config{
		Waybar: map[string]any{},
		Menus: map[string][]MenuItem{
			"custom/Good": {{Name: "Terminal", Action: "foot"}, {Name: "Browser", Action: "firefox"}},
			"custom/Bad":  {{Name: "OnlyOne", Action: "true"}},
		},
	}
	results := buildXML(cfg)

	if _, ok := results["custom/Good"]; !ok {
		t.Error("custom/Good should be present in results")
	}
	if _, ok := results["custom/Bad"]; ok {
		t.Error("custom/Bad should be absent from results")
	}
	goodPath := filepath.Join(menuDir, "custom-good.xml")
	if _, err := os.Stat(goodPath); err != nil {
		t.Errorf("custom/Good XML file should be written: %v", err)
	}
	badPath := filepath.Join(menuDir, "custom-bad.xml")
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Errorf("custom/Bad XML file should NOT be written, err = %v", err)
	}
}

func TestAddNodeSeparator(t *testing.T) {
	dir := t.TempDir()
	origMenuDir := menuDir
	menuDir = filepath.Join(dir, "gtk3-menus")
	defer func() { menuDir = origMenuDir }()

	cfg := Config{
		Waybar: map[string]any{},
		Menus: map[string][]MenuItem{
			"custom/Start": {
				{Name: "Terminal", Action: "foot"},
				{Name: "---"},
				{Name: "Browser", Action: "firefox"},
			},
		},
	}
	menus := buildXML(cfg)
	data, err := os.ReadFile(menus["custom/Start"].path)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(data)
	if !strings.Contains(xml, "GtkSeparatorMenuItem") {
		t.Errorf("expected GtkSeparatorMenuItem in XML, got:\n%s", xml)
	}
	if strings.Contains(xml, `id="---"`) {
		t.Errorf("separator must not use id=--- in XML:\n%s", xml)
	}
	if _, ok := menus["custom/Start"].actions["---"]; ok {
		t.Errorf("separator must not appear in actions map")
	}
}

func TestAddNodeDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	origMenuDir := menuDir
	menuDir = filepath.Join(dir, "gtk3-menus")
	defer func() { menuDir = origMenuDir }()

	cfg := Config{
		Waybar: map[string]any{},
		Menus: map[string][]MenuItem{
			"custom/Start": {
				{Name: "Firefox", Action: "firefox"},
				{Name: "Firefox", Action: "firefox --private-window"},
				{Name: "Firefox", Action: "firefox --new-tab"},
			},
		},
	}
	menus := buildXML(cfg)
	data, err := os.ReadFile(menus["custom/Start"].path)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(data)
	actions := menus["custom/Start"].actions

	for _, wantID := range []string{"Firefox", "Firefox_2", "Firefox_3"} {
		if !strings.Contains(xml, `id="`+wantID+`"`) {
			t.Errorf("expected id=%q in XML:\n%s", wantID, xml)
		}
		if _, ok := actions[wantID]; !ok {
			t.Errorf("expected actions[%q] to be set", wantID)
		}
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
