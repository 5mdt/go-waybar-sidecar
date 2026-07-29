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
		filepath.Join(gwsDir, "config.yaml"):      true,
		filepath.Join(gwsDir, "foo.yml"):          true,
		filepath.Join(gwsDir, "config.json"):      false,
		filepath.Join(gwsDir, ".config.yaml.swp"): false,
		filepath.Join(gwsDir, "config.yaml~"):     false,
	}
	for name, want := range gwsCases {
		if got := isWatchedFile(name); got != want {
			t.Errorf("isWatchedFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsWatchedFile_AppDir(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_DATA_DIRS", "")

	appDir := filepath.Join(dataHome, "applications")

	origGwsDir := gwsDir
	gwsDir = t.TempDir()
	defer func() { gwsDir = origGwsDir }()

	cases := map[string]bool{
		filepath.Join(appDir, "foo.desktop"):        true,
		filepath.Join(appDir, "sub", "foo.desktop"): true,
		filepath.Join(appDir, "foo.json"):           false,
		filepath.Join(gwsDir, "foo.desktop"):        false,
	}
	for name, want := range cases {
		if got := isWatchedFile(name); got != want {
			t.Errorf("isWatchedFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSymlinkTargets(t *testing.T) {
	dir := t.TempDir()
	origWaybarDir, origGwsDir := waybarDir, gwsDir
	waybarDir = filepath.Join(dir, "waybar")
	gwsDir = filepath.Join(dir, "gws")
	defer func() {
		waybarDir, gwsDir = origWaybarDir, origGwsDir
	}()

	external := filepath.Join(dir, "external")
	for _, d := range []string{waybarDir, gwsDir, external} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	realStyle := filepath.Join(external, "real-style.css")
	realConfig := filepath.Join(external, "real-config.yaml")
	realIgnored := filepath.Join(external, "real-ignored.txt")
	for _, f := range []string{realStyle, realConfig, realIgnored} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Whitelisted symlinks.
	if err := os.Symlink(realStyle, filepath.Join(waybarDir, "style.css")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realConfig, filepath.Join(gwsDir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	// Symlink to a non-whitelisted extension: should be ignored.
	if err := os.Symlink(realIgnored, filepath.Join(waybarDir, "ignored.txt")); err != nil {
		t.Fatal(err)
	}
	// Regular (non-symlink) whitelisted file: should be ignored by symlinkTargets.
	if err := os.WriteFile(filepath.Join(waybarDir, "regular.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	targets := symlinkTargets(gwsDir, waybarDir)

	wantStyle, err := filepath.EvalSymlinks(realStyle)
	if err != nil {
		t.Fatal(err)
	}
	wantConfig, err := filepath.EvalSymlinks(realConfig)
	if err != nil {
		t.Fatal(err)
	}

	if len(targets) != 2 {
		t.Fatalf("symlinkTargets() = %v, want exactly 2 entries", targets)
	}
	if !targets[wantStyle] {
		t.Errorf("expected resolved style target %q in %v", wantStyle, targets)
	}
	if !targets[wantConfig] {
		t.Errorf("expected resolved config target %q in %v", wantConfig, targets)
	}
}

func TestAcceptEvent(t *testing.T) {
	dir := t.TempDir()
	origWaybarDir, origGwsDir := waybarDir, gwsDir
	origMenuDir, origOutJSON := menuDir, outJSON
	waybarDir = dir
	gwsDir = filepath.Join(dir, "go-waybar-sidecar")
	menuDir = filepath.Join(dir, "gtk3-menus")
	outJSON = filepath.Join(dir, "config.jsonc")
	defer func() {
		waybarDir, gwsDir = origWaybarDir, origGwsDir
		menuDir, outJSON = origMenuDir, origOutJSON
	}()

	targets := map[string]bool{
		filepath.Join(dir, "external", "real-style.css"): true,
	}

	cases := map[string]bool{
		filepath.Join(dir, "external", "real-style.css"): true,  // known symlink target
		filepath.Join(dir, "style.css"):                  true,  // regular whitelisted file
		filepath.Join(dir, "config.yaml"):                false, // not whitelisted in waybarDir
		outJSON:                                          false, // self-generated output
		filepath.Join(menuDir, "foo.xml"):                false, // self-generated menu
	}
	for name, want := range cases {
		if got := acceptEvent(name, targets); got != want {
			t.Errorf("acceptEvent(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCollectWatchDirs(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	external := filepath.Join(dir, "external")

	// root/a is a real subdirectory.
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	// external/b is a real subdirectory, reached via root/link -> external.
	if err := os.MkdirAll(filepath.Join(external, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	// root/loop -> root: a cycle back to an ancestor must not recurse forever
	// or produce duplicate entries.
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}

	got := collectWatchDirs(root)

	want := map[string]bool{
		root:                             true,
		filepath.Join(root, "a"):         true,
		filepath.Join(root, "link"):      true,
		filepath.Join(root, "link", "b"): true,
	}
	if len(got) != len(want) {
		t.Fatalf("collectWatchDirs(%q) = %v, want exactly %v", root, got, want)
	}
	seen := make(map[string]bool)
	for _, d := range got {
		if seen[d] {
			t.Errorf("collectWatchDirs(%q) returned duplicate %q", root, d)
		}
		seen[d] = true
		if !want[d] {
			t.Errorf("collectWatchDirs(%q) returned unexpected %q", root, d)
		}
	}
	for d := range want {
		if !seen[d] {
			t.Errorf("collectWatchDirs(%q) missing %q", root, d)
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
