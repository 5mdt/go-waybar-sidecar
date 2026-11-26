package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestWriteJSONUnchangedIsNotRewritten(t *testing.T) {
	dir := t.TempDir()
	origOutJSON := outJSON
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() { outJSON = origOutJSON }()

	cfg := map[string]any{"clock": map[string]any{"format": "same"}}
	writeJSON(cfg, nil)
	before, err := os.Stat(outJSON)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	writeJSON(cfg, nil)

	after, _ := os.Stat(outJSON)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("identical output was rewritten")
	}
	if _, err := os.Stat(outJSON + ".bak"); !os.IsNotExist(err) {
		t.Errorf("unexpected .bak for unchanged output, err = %v", err)
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

	for _, wantID := range []string{"item1", "item2", "item3"} {
		if !strings.Contains(xml, `id="`+wantID+`"`) {
			t.Errorf("expected id=%q in XML:\n%s", wantID, xml)
		}
		if _, ok := actions[wantID]; !ok {
			t.Errorf("expected actions[%q] to be set", wantID)
		}
	}
}

func TestBuildXMLIDsNeverCollide(t *testing.T) {
	xml := buildIconMenu(t, []MenuItem{
		{Name: "Foo", Action: "a"},
		{Name: "Foo", Action: "b"},
		{Name: "Foo_2", Action: "c"},
		{Name: "sep1", Action: "d"},
		{Name: "---"},
	})
	if !strings.Contains(xml, `id="sep5"`) || !strings.Contains(xml, `id="item4"`) {
		t.Errorf("expected counter-based ids:\n%s", xml)
	}
}

func TestAddNodeBoldEscapesMarkup(t *testing.T) {
	root := etree.NewElement("menu")
	n := 0
	addNode(root, MenuItem{Name: "A & <B>", Action: "x", Bold: true}, map[string]string{}, &n, false)

	var label string
	for _, p := range root.FindElements("//object[@class='GtkLabel']/property[@name='label']") {
		label = p.Text()
	}
	if label != "<b>A &amp; &lt;B&gt;</b>" {
		t.Errorf("label = %q, want escaped markup", label)
	}
}

func TestAddNodeBold(t *testing.T) {
	dir := t.TempDir()
	origMenuDir := menuDir
	menuDir = filepath.Join(dir, "gtk3-menus")
	defer func() { menuDir = origMenuDir }()

	cfg := Config{
		Waybar: map[string]any{},
		Menus: map[string][]MenuItem{
			"custom/Power": {
				{Name: "● Balanced", Action: "powerprofilesctl set balanced", Bold: true},
				{Name: "  Performance", Action: "powerprofilesctl set performance"},
			},
		},
	}
	menus := buildXML(cfg)
	data, err := os.ReadFile(menus["custom/Power"].path)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(data)

	if !strings.Contains(xml, `class="GtkLabel"`) {
		t.Errorf("expected a GtkLabel child for the bold item, got:\n%s", xml)
	}
	if !strings.Contains(xml, "use-markup") {
		t.Errorf("expected use-markup property, got:\n%s", xml)
	}
	if !strings.Contains(xml, "&lt;b&gt;● Balanced&lt;/b&gt;") {
		t.Errorf("expected bold markup text, got:\n%s", xml)
	}

	actions := menus["custom/Power"].actions
	if actions["item1"] != "powerprofilesctl set balanced" {
		t.Errorf("actions[item1] = %q, want %q", actions["item1"], "powerprofilesctl set balanced")
	}
	if actions["item2"] != "powerprofilesctl set performance" {
		t.Errorf("actions[item2] = %q, want %q", actions["item2"], "powerprofilesctl set performance")
	}

	if err := validateMenuDoc(newDocFromFile(t, menus["custom/Power"].path)); err != nil {
		t.Errorf("expected bold-item menu to pass validation, got: %v", err)
	}
}

func newDocFromFile(t *testing.T, path string) *etree.Document {
	t.Helper()
	doc := etree.NewDocument()
	if err := doc.ReadFromFile(path); err != nil {
		t.Fatal(err)
	}
	return doc
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

func buildIconMenu(t *testing.T, items []MenuItem) string {
	t.Helper()
	origMenuDir := menuDir
	menuDir = filepath.Join(t.TempDir(), "gtk3-menus")
	defer func() { menuDir = origMenuDir }()

	menus := buildXML(Config{Waybar: map[string]any{}, Menus: map[string][]MenuItem{"custom/Start": items}})
	path := menus["custom/Start"].path
	if err := validateMenuDoc(newDocFromFile(t, path)); err != nil {
		t.Errorf("menu failed validation: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAddNodeIcon(t *testing.T) {
	xml := buildIconMenu(t, []MenuItem{
		{Name: "Firefox", Action: "firefox", Icon: "firefox"},
		{Name: "Plain", Action: "plain"},
		{Name: "Gone", Action: "gone", Icon: "/nonexistent/icon.png"},
	})

	for _, want := range []string{`class="GtkBox"`, `class="GtkImage"`, `id="item1_icon"`, `id="item2_icon"`, `id="item3_icon"`, `class="GtkLabel"`} {
		if !strings.Contains(xml, want) {
			t.Errorf("expected %s in XML:\n%s", want, xml)
		}
	}
	if strings.Contains(xml, `name="gicon"`) || strings.Contains(xml, `name="file"`) {
		t.Errorf("expected only one icon-name property, no gicon/file:\n%s", xml)
	}
	if n := strings.Count(xml, `name="icon-name"`); n != 1 {
		t.Errorf("icon-name count = %d, want 1 (only the usable icon):\n%s", n, xml)
	}
	if !strings.Contains(xml, `"icon-name">firefox</property>`) {
		t.Errorf("expected icon-name firefox:\n%s", xml)
	}
	if n := strings.Count(xml, `name="pixel-size"`); n != 3 {
		t.Errorf("pixel-size count = %d, want 3 (blank slots too):\n%s", n, xml)
	}
}

func TestAddNodeIconExistingPath(t *testing.T) {
	icon := filepath.Join(t.TempDir(), "i.png")
	if err := os.WriteFile(icon, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	xml := buildIconMenu(t, []MenuItem{{Name: "A", Action: "a", Icon: icon}})
	if !strings.Contains(xml, `"file">`+icon+"</property>") {
		t.Errorf("expected file %s:\n%s", icon, xml)
	}
}

func TestAddNodeIconBold(t *testing.T) {
	xml := buildIconMenu(t, []MenuItem{{Name: "B", Action: "b", Icon: "x", Bold: true}})
	if !strings.Contains(xml, "&lt;b&gt;B&lt;/b&gt;") || !strings.Contains(xml, "use-markup") {
		t.Errorf("expected bold markup label in icon layout:\n%s", xml)
	}
}

func TestAddNodeNoIconsUnchanged(t *testing.T) {
	xml := buildIconMenu(t, []MenuItem{{Name: "A", Action: "a"}})
	if strings.Contains(xml, "GtkBox") || strings.Contains(xml, "GtkImage") {
		t.Errorf("expected no icon layout without icons:\n%s", xml)
	}
}

func TestAddNodeIconGlyph(t *testing.T) {
	xml := buildIconMenu(t, []MenuItem{
		{Name: "Launcher", Action: "l", Icon: "\U000F0463"},
		{Name: "Plain", Action: "p"},
	})
	if !strings.Contains(xml, `<object class="GtkLabel" id="item1_icon">`) {
		t.Errorf("expected GtkLabel icon slot:\n%s", xml)
	}
	if !strings.Contains(xml, ">\U000F0463</property>") || !strings.Contains(xml, `name="width-request">16<`) {
		t.Errorf("expected glyph text and width-request 16:\n%s", xml)
	}
	if strings.Contains(xml, `class="GtkImage" id="item1_icon"`) {
		t.Errorf("glyph item must not have a GtkImage:\n%s", xml)
	}
	if strings.Contains(xml, `name="icon-name"`) || strings.Contains(xml, `name="file"`) {
		t.Errorf("glyph must not set icon-name/file:\n%s", xml)
	}
	if !strings.Contains(xml, `class="GtkImage" id="item2_icon"`) {
		t.Errorf("sibling should keep the blank GtkImage slot:\n%s", xml)
	}
}

func TestJSONSafeDeepCopiesAndStringifiesKeys(t *testing.T) {
	in := map[string]any{
		"a": map[interface{}]interface{}{1: []any{map[interface{}]interface{}{true: "x"}}},
	}
	out := jsonSafe(in).(map[string]any)

	inner := out["a"].(map[string]any)["1"].([]any)[0].(map[string]any)
	if inner["true"] != "x" {
		t.Errorf("keys not stringified: %v", out)
	}
	if _, ok := in["a"].(map[interface{}]interface{}); !ok {
		t.Error("input was modified")
	}
	out["b"] = 1
	if _, ok := in["b"]; ok {
		t.Error("output shares the top-level map with input")
	}
}

func TestWriteJSONDoesNotModifyInput(t *testing.T) {
	dir := t.TempDir()
	origOutJSON := outJSON
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() { outJSON = origOutJSON }()

	cfg := map[string]any{"custom/Start": map[string]any{
		"menu_definition": []any{map[string]any{"name": "x"}},
	}}
	writeJSON(cfg, map[string]menuResult{"custom/Start": {path: "/m.xml", actions: map[string]string{}}})

	mod := cfg["custom/Start"].(map[string]any)
	if _, ok := mod["menu_definition"]; !ok {
		t.Error("menu_definition removed from input")
	}
	if _, ok := mod["menu-file"]; ok {
		t.Error("menu-file injected into input")
	}
}
