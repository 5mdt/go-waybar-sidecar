package main

import (
	"flag"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// parseTestFlags builds the real flag set and parses args into it.
func parseTestFlags(t *testing.T, args ...string) *flag.FlagSet {
	t.Helper()
	fs := newFlagSet()
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	return fs
}

// clearSettingsEnv unsets every settings env var for the test's duration.
func clearSettingsEnv(t *testing.T) {
	t.Helper()
	for _, s := range settings {
		t.Setenv(s.env, "")
		os.Unsetenv(s.env)
	}
	t.Setenv(configEnv, "")
	os.Unsetenv(configEnv)
}

// loadSettings tests

func TestLoadSettings_ValidSidecarSection(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, cfgFile, `
sidecar:
  log-level: debug
  log-output: /tmp/app.log
  notify-level: warn
  no-generate: true
  daemon: yes
waybar:
  layer: top
`)
	got, err := loadSettings(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"log-level": "debug", "log-output": "/tmp/app.log", "notify-level": "warn",
		"no-generate": "true", "daemon": "yes",
	}
	if !maps.Equal(got, want) {
		t.Errorf("loadSettings() = %v, want %v", got, want)
	}
}

func TestLoadSettings_MissingFileOrKey(t *testing.T) {
	got, err := loadSettings(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil || len(got) != 0 {
		t.Errorf("missing file: got %v, %v; want empty, nil", got, err)
	}

	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, cfgFile, "waybar:\n  layer: top\n")
	got, err = loadSettings(cfgFile)
	if err != nil || len(got) != 0 {
		t.Errorf("no sidecar key: got %v, %v; want empty, nil", got, err)
	}
}

func TestLoadSettings_MalformedYAML(t *testing.T) {
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, cfgFile, "sidecar: [unclosed\n")
	if _, err := loadSettings(cfgFile); err == nil {
		t.Error("expected an error for malformed YAML")
	}
}

// resolveConfigPath tests

func TestResolveConfigPath_Precedence(t *testing.T) {
	orig := configPath
	configPath = "/default/config.yaml"
	defer func() { configPath = orig }()
	clearSettingsEnv(t)

	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	flagPath := filepath.Join(dir, "flag.yaml")

	if got := resolveConfigPath(parseTestFlags(t)); got != configPath {
		t.Errorf("default: got %q, want %q", got, configPath)
	}

	t.Setenv(configEnv, envPath)
	if got := resolveConfigPath(parseTestFlags(t)); got != envPath {
		t.Errorf("env: got %q, want %q", got, envPath)
	}
	if got := resolveConfigPath(parseTestFlags(t, "--config", flagPath)); got != flagPath {
		t.Errorf("flag: got %q, want %q", got, flagPath)
	}
}

// resolveSettings tests

func TestResolveSettings_Defaults(t *testing.T) {
	clearSettingsEnv(t)
	r, err := resolveSettings(parseTestFlags(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := resolvedSettings{
		logLevel: defaultLogLevel, logOutput: defaultLogOutput, notifyLevel: defaultNotifyLevel,
		noGenerate: defaultNoGenerate, daemon: defaultDaemon,
	}
	if r != want {
		t.Errorf("got %+v, want %+v", r, want)
	}
}

// TestResolveSettings_Precedence checks every setting through every layer:
// each higher layer wins over the lower ones.
func TestResolveSettings_Precedence(t *testing.T) {
	for _, s := range settings {
		file, env, flagV := "file-value", "env-value", "flag-value"
		if s.isBool {
			file, env, flagV = "true", "yes", "false"
		}
		t.Run(s.key, func(t *testing.T) {
			layers := []struct {
				name string
				file map[string]string
				env  string
				args []string
				want string
			}{
				{"file over default", map[string]string{s.key: file}, "", nil, file},
				{"env over file", map[string]string{s.key: file}, env, nil, env},
				{"flag over env", map[string]string{s.key: file}, env, []string{"--" + s.key + "=" + flagV}, flagV},
			}
			for _, l := range layers {
				clearSettingsEnv(t)
				if l.env != "" {
					t.Setenv(s.env, l.env)
				}
				r, err := resolveSettings(parseTestFlags(t, l.args...), l.file)
				if err != nil {
					t.Fatalf("%s: %v", l.name, err)
				}
				if got := settingValue(r, s.key); got != normalize(s, l.want) {
					t.Errorf("%s: got %q, want %q", l.name, got, normalize(s, l.want))
				}
			}
		})
	}
}

// settingValue reads one resolved field back as the string the table row uses.
func settingValue(r resolvedSettings, key string) string {
	switch key {
	case "log-level":
		return r.logLevel
	case "log-output":
		return r.logOutput
	case "notify-level":
		return r.notifyLevel
	case "no-generate":
		return strconv.FormatBool(r.noGenerate)
	case "daemon":
		return strconv.FormatBool(r.daemon)
	}
	return "?"
}

func normalize(s setting, v string) string {
	if !s.isBool {
		return v
	}
	b, _ := parseBoolEnv(v)
	return strconv.FormatBool(b)
}

func TestResolveSettings_BoolValues(t *testing.T) {
	cases := map[string]bool{
		"true": true, "false": false, "1": true, "0": false, "TRUE": true,
		"FALSE": false, "yes": true, "no": false, "YES": true, "No": false,
	}
	for val, want := range cases {
		t.Run(val, func(t *testing.T) {
			clearSettingsEnv(t)
			t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", val)
			r, err := resolveSettings(parseTestFlags(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			if r.noGenerate != want {
				t.Errorf("NO_GENERATE=%q: got %v, want %v", val, r.noGenerate, want)
			}
		})
	}
}

func TestResolveSettings_InvalidBoolIsError(t *testing.T) {
	clearSettingsEnv(t)
	t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", "yes-please")
	if _, err := resolveSettings(parseTestFlags(t), nil); err == nil {
		t.Error("expected an error for an invalid env bool")
	}

	clearSettingsEnv(t)
	if _, err := resolveSettings(parseTestFlags(t), map[string]string{"daemon": "maybe"}); err == nil {
		t.Error("expected an error for an invalid file bool")
	}
}

// ensureSettings tests

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEnsureSettings_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	if err := ensureSettings(path); err != nil {
		t.Errorf("ensureSettings on missing file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected no file to be created for missing input")
	}
}

func TestEnsureSettings_AllKeysAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "waybar:\n  layer: top\n  clock:\n    format: \"{:%H:%M}\"\n"
	writeFile(t, path, original)

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	content := readFile(t, path)
	for _, key := range []string{"log-level", "log-output", "notify-level", "no-generate", "daemon"} {
		if !strings.Contains(content, key+":") {
			t.Errorf("expected key %q in output, got:\n%s", key, content)
		}
	}
	// waybar section preserved
	if !strings.Contains(content, "clock:") {
		t.Errorf("waybar section lost after ensureSettings:\n%s", content)
	}

	// backup created
	bak := path + ".bak"
	if bakContent := readFile(t, bak); bakContent != original {
		t.Errorf("backup content = %q, want %q", bakContent, original)
	}
}

func TestEnsureSettings_PartialSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "sidecar:\n  log-level: debug\nwaybar:\n  layer: top\n")

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	content := readFile(t, path)
	// user value preserved
	if !strings.Contains(content, "log-level: debug") {
		t.Errorf("user log-level overwritten; got:\n%s", content)
	}
	// missing keys added
	for _, key := range []string{"log-output", "notify-level", "no-generate", "daemon"} {
		if !strings.Contains(content, key+":") {
			t.Errorf("expected key %q added, got:\n%s", key, content)
		}
	}
}

func TestEnsureSettings_AllKeysPresent_NoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	full := "sidecar:\n  log-level: info\n  log-output: stdout\n  notify-level: error\n  no-generate: false\n  daemon: false\nwaybar:\n  layer: top\n"
	writeFile(t, path, full)

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	// file must not be changed
	if got := readFile(t, path); got != full {
		t.Errorf("file changed when it shouldn't have:\ngot:  %q\nwant: %q", got, full)
	}
	// no backup
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("unexpected .bak created when no keys were added")
	}
}

func TestEnsureSettings_UserNonDefaultValuePreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "sidecar:\n  daemon: true\n")

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "daemon: true") {
		t.Errorf("user daemon:true was overwritten; got:\n%s", content)
	}
}

func TestEnsureSettings_CommentPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "# my config\nwaybar:\n  layer: top\n")

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	content := readFile(t, path)
	if !strings.Contains(content, "# my config") {
		t.Errorf("comment lost after ensureSettings:\n%s", content)
	}
}

func TestEnsureSettings_DefaultValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "waybar:\n  layer: top\n")

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	content := readFile(t, path)
	for _, wantSubstr := range []string{
		"log-level: " + defaultLogLevel,
		"log-output: " + defaultLogOutput,
		"notify-level: " + defaultNotifyLevel,
	} {
		if !strings.Contains(content, wantSubstr) {
			t.Errorf("expected %q in output:\n%s", wantSubstr, content)
		}
	}
}

func TestEnsureSettings_AllTableKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, "waybar:\n  layer: top\n")

	if err := ensureSettings(path); err != nil {
		t.Fatalf("ensureSettings: %v", err)
	}

	got, err := loadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range settings {
		if got[s.key] != s.def {
			t.Errorf("%s = %q, want default %q", s.key, got[s.key], s.def)
		}
	}
}
