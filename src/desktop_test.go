package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppDirs_Defaults(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_DATA_DIRS", "")

	home := os.Getenv("HOME")
	want := []string{
		filepath.Join(home, ".local/share/applications"),
		"/usr/local/share/applications",
		"/usr/share/applications",
	}

	got := appDirs()
	if len(got) != len(want) {
		t.Fatalf("appDirs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("appDirs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAppDirs_FromEnv(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/custom/data")
	t.Setenv("XDG_DATA_DIRS", "/opt/a:/opt/b")

	want := []string{
		"/custom/data/applications",
		"/opt/a/applications",
		"/opt/b/applications",
	}

	got := appDirs()
	if len(got) != len(want) {
		t.Fatalf("appDirs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("appDirs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseDesktopEntry_Basic(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=Firefox\nExec=firefox %u\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("parseDesktopEntry() ok = false, want true")
	}
	if e.name != "Firefox" {
		t.Errorf("name = %q, want %q", e.name, "Firefox")
	}
	if e.exec != "firefox %u" {
		t.Errorf("exec = %q, want %q", e.exec, "firefox %u")
	}
}

func TestParseDesktopEntry_SkipsNoDisplay(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=Hidden\nExec=foo\nNoDisplay=true\n")
	if _, ok := parseDesktopEntry(b); ok {
		t.Fatal("parseDesktopEntry() ok = true, want false for NoDisplay=true")
	}
}

func TestParseDesktopEntry_SkipsHidden(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=Hidden\nExec=foo\nHidden=true\n")
	if _, ok := parseDesktopEntry(b); ok {
		t.Fatal("parseDesktopEntry() ok = true, want false for Hidden=true")
	}
}

func TestParseDesktopEntry_SkipsNonApplication(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Link\nName=Somewhere\nExec=foo\n")
	if _, ok := parseDesktopEntry(b); ok {
		t.Fatal("parseDesktopEntry() ok = true, want false for Type=Link")
	}
}

func TestParseDesktopEntry_SkipsMissingNameOrExec(t *testing.T) {
	cases := []string{
		"[Desktop Entry]\nType=Application\nExec=foo\n",
		"[Desktop Entry]\nType=Application\nName=NoExec\n",
	}
	for _, b := range cases {
		if _, ok := parseDesktopEntry([]byte(b)); ok {
			t.Errorf("parseDesktopEntry(%q) ok = true, want false", b)
		}
	}
}

func TestParseDesktopEntry_IgnoresOtherGroups(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=App\nExec=app\n\n[Desktop Action New]\nName=New Window\nExec=app --new\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("parseDesktopEntry() ok = false, want true")
	}
	if e.name != "App" || e.exec != "app" {
		t.Errorf("got %+v, want name=App exec=app (action group keys must not leak in)", e)
	}
}

func TestParseDesktopEntry_IgnoresLocalizedName(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=App\nName[de]=Anwendung\nExec=app\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("parseDesktopEntry() ok = false, want true")
	}
	if e.name != "App" {
		t.Errorf("name = %q, want %q (localized key must not win)", e.name, "App")
	}
}

func TestStripFieldCodes(t *testing.T) {
	cases := map[string]string{
		"firefox %u":       "firefox",
		"app %f %F":        "app",
		"app --flag %U":    "app --flag",
		"echo 100%%":       "echo 100%",
		"app %i --name %c": "app --name",
		"plain command":    "plain command",
		"app %k":           "app",
		"echo %%f":         "echo %f",
	}
	for input, want := range cases {
		if got := stripFieldCodes(input); got != want {
			t.Errorf("stripFieldCodes(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseDesktopEntry_VisibilityKeys(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=A\nExec=a\nTerminal=true\n" +
		"OnlyShowIn=GNOME;KDE;\nNotShowIn=XFCE;\nTryExec=a-bin\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if !e.terminal || e.tryExec != "a-bin" ||
		!slices.Equal(e.onlyShowIn, []string{"GNOME", "KDE"}) || !slices.Equal(e.notShowIn, []string{"XFCE"}) {
		t.Errorf("unexpected entry: %+v", e)
	}
}

func TestEntryVisible(t *testing.T) {
	cases := []struct {
		name     string
		e        desktopEntry
		desktops []string
		want     bool
	}{
		{"no restrictions", desktopEntry{}, []string{"sway"}, true},
		{"only show in match", desktopEntry{onlyShowIn: []string{"GNOME", "sway"}}, []string{"sway"}, true},
		{"only show in no match", desktopEntry{onlyShowIn: []string{"GNOME"}}, []string{"sway"}, false},
		{"only show in unknown desktop", desktopEntry{onlyShowIn: []string{"GNOME"}}, nil, false},
		{"not show in match", desktopEntry{notShowIn: []string{"sway"}}, []string{"sway"}, false},
		{"not show in no match", desktopEntry{notShowIn: []string{"GNOME"}}, []string{"sway"}, true},
		{"try exec missing", desktopEntry{tryExec: "definitely-not-installed-xyz"}, nil, false},
		{"try exec present", desktopEntry{tryExec: "sh"}, nil, true},
	}
	for _, c := range cases {
		if got := entryVisible(c.e, c.desktops); got != c.want {
			t.Errorf("%s: entryVisible = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestScanApplications_TerminalSortAndVisibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())
	t.Setenv("XDG_CURRENT_DESKTOP", "sway")
	apps := filepath.Join(home, "applications")

	writeDesktopFile(t, apps, "zoom.desktop", "[Desktop Entry]\nType=Application\nName=Zoom\nExec=zoom\n")
	writeDesktopFile(t, apps, "gimp.desktop", "[Desktop Entry]\nType=Application\nName=gimp\nExec=gimp %U\n")
	writeDesktopFile(t, apps, "htop.desktop", "[Desktop Entry]\nType=Application\nName=Htop\nExec=htop\nTerminal=true\n")
	writeDesktopFile(t, apps, "gnome.desktop", "[Desktop Entry]\nType=Application\nName=GnomeOnly\nExec=g\nOnlyShowIn=GNOME;\n")

	items := scanApplications()
	var got []string
	for _, it := range items {
		got = append(got, it.Name+"="+it.Action)
	}
	want := []string{"gimp=gimp", "Htop=xdg-terminal-exec htop", "Zoom=zoom"}
	if !slices.Equal(got, want) {
		t.Errorf("items = %v, want %v", got, want)
	}
}

func writeDesktopFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanApplications_SortedAndDeduped(t *testing.T) {
	high := t.TempDir()
	low := t.TempDir()
	t.Setenv("XDG_DATA_HOME", high)
	t.Setenv("XDG_DATA_DIRS", low)

	writeDesktopFile(t, filepath.Join(high, "applications"), "zeta.desktop",
		"[Desktop Entry]\nType=Application\nName=Zeta\nExec=zeta\n")
	writeDesktopFile(t, filepath.Join(high, "applications"), "app.desktop",
		"[Desktop Entry]\nType=Application\nName=High Priority\nExec=high\n")
	writeDesktopFile(t, filepath.Join(low, "applications"), "app.desktop",
		"[Desktop Entry]\nType=Application\nName=Low Priority\nExec=low\n")
	writeDesktopFile(t, filepath.Join(low, "applications"), "alpha.desktop",
		"[Desktop Entry]\nType=Application\nName=Alpha\nExec=alpha\n")

	items := scanApplications()
	if len(items) != 3 {
		t.Fatalf("scanApplications() = %d items, want 3: %+v", len(items), items)
	}

	names := []string{items[0].Name, items[1].Name, items[2].Name}
	want := []string{"Alpha", "High Priority", "Zeta"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("items[%d].Name = %q, want %q (got order %v)", i, names[i], want[i], names)
		}
	}

	for _, item := range items {
		if item.Name == "High Priority" && item.Action != "high" {
			t.Errorf("expected high-precedence dir to win dedupe, got action %q", item.Action)
		}
		if item.Name == "Low Priority" {
			t.Error("low-precedence duplicate should have been deduped out")
		}
	}
}

func TestScanApplications_Recursive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())

	writeDesktopFile(t, filepath.Join(home, "applications"), filepath.Join("sub", "app.desktop"),
		"[Desktop Entry]\nType=Application\nName=Nested\nExec=nested\n")

	items := scanApplications()
	if len(items) != 1 {
		t.Fatalf("scanApplications() = %d items, want 1: %+v", len(items), items)
	}
	if items[0].Name != "Nested" {
		t.Errorf("Name = %q, want %q", items[0].Name, "Nested")
	}
}

func TestScanApplications_MissingDirIgnored(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("XDG_DATA_DIRS", filepath.Join(t.TempDir(), "also-missing"))

	items := scanApplications()
	if len(items) != 0 {
		t.Fatalf("scanApplications() = %v, want empty for missing dirs", items)
	}
}

func TestParseDesktopEntry_Categories(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=A\nExec=a\nCategories=Network;WebBrowser; GTK ;;\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("parseDesktopEntry() ok = false, want true")
	}
	want := []string{"Network", "WebBrowser", "GTK"}
	if strings.Join(e.categories, ",") != strings.Join(want, ",") {
		t.Errorf("categories = %v, want %v", e.categories, want)
	}

	e, _ = parseDesktopEntry([]byte("[Desktop Entry]\nType=Application\nName=A\nExec=a\n"))
	if len(e.categories) != 0 {
		t.Errorf("categories = %v, want none", e.categories)
	}
}

func TestScanApplications_Categories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	t.Setenv("XDG_DATA_DIRS", t.TempDir())

	writeDesktopFile(t, filepath.Join(home, "applications"), "a.desktop",
		"[Desktop Entry]\nType=Application\nName=A\nExec=a\nCategories=Utility;Development;\n")

	items := scanApplications()
	if len(items) != 1 || strings.Join(items[0].Categories, ",") != "Utility,Development" {
		t.Fatalf("scanApplications() = %+v, want Categories [Utility Development]", items)
	}
}

func TestParseDesktopEntry_Icon(t *testing.T) {
	b := []byte("[Desktop Entry]\nType=Application\nName=Firefox\nExec=firefox\nIcon=firefox\n")
	e, ok := parseDesktopEntry(b)
	if !ok {
		t.Fatal("parseDesktopEntry() ok = false, want true")
	}
	if e.icon != "firefox" {
		t.Errorf("icon = %q, want %q", e.icon, "firefox")
	}
}
