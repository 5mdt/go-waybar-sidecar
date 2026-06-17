package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadSettings tests

func TestLoadSettings_ValidSidecarSection(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := `
sidecar:
  log-level: debug
  log-output: /tmp/app.log
  notify-level: warn
  no-generate: true
  daemon: true
waybar:
  layer: top
`
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings(cfgFile)
	if s.LogLevel == nil || *s.LogLevel != "debug" {
		t.Errorf("LogLevel: got %v, want debug", s.LogLevel)
	}
	if s.LogOutput == nil || *s.LogOutput != "/tmp/app.log" {
		t.Errorf("LogOutput: got %v, want /tmp/app.log", s.LogOutput)
	}
	if s.NotifyLevel == nil || *s.NotifyLevel != "warn" {
		t.Errorf("NotifyLevel: got %v, want warn", s.NotifyLevel)
	}
	if s.NoGenerate == nil || !*s.NoGenerate {
		t.Errorf("NoGenerate: got %v, want true", s.NoGenerate)
	}
	if s.Daemon == nil || !*s.Daemon {
		t.Errorf("Daemon: got %v, want true", s.Daemon)
	}
}

func TestLoadSettings_MissingFile(t *testing.T) {
	s := loadSettings(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if s.LogLevel != nil || s.LogOutput != nil || s.NotifyLevel != nil || s.NoGenerate != nil || s.Daemon != nil {
		t.Error("expected all-nil sidecarSettings for missing file")
	}
}

func TestLoadSettings_NoSidecarKey(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("waybar:\n  layer: top\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings(cfgFile)
	if s.LogLevel != nil || s.LogOutput != nil || s.NotifyLevel != nil || s.NoGenerate != nil || s.Daemon != nil {
		t.Error("expected all-nil sidecarSettings when sidecar: key is absent")
	}
}

func TestLoadSettings_PartialSidecarSection(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	content := "sidecar:\n  log-level: warn\n"
	if err := os.WriteFile(cfgFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := loadSettings(cfgFile)
	if s.LogLevel == nil || *s.LogLevel != "warn" {
		t.Errorf("LogLevel: got %v, want warn", s.LogLevel)
	}
	if s.LogOutput != nil {
		t.Errorf("LogOutput: expected nil for absent key, got %v", s.LogOutput)
	}
}

// resolveConfigPath tests

func TestResolveConfigPath_Default(t *testing.T) {
	orig := configPath
	configPath = "/default/config.yaml"
	defer func() { configPath = orig }()

	t.Setenv("GO_WAYBAR_SIDECAR_CONFIG", "")
	os.Unsetenv("GO_WAYBAR_SIDECAR_CONFIG")

	got := resolveConfigPath(cliFlags{}, map[string]bool{})
	if got != configPath {
		t.Errorf("got %q, want %q", got, configPath)
	}
}

func TestResolveConfigPath_Env(t *testing.T) {
	orig := configPath
	configPath = "/default/config.yaml"
	defer func() { configPath = orig }()

	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	t.Setenv("GO_WAYBAR_SIDECAR_CONFIG", envPath)

	got := resolveConfigPath(cliFlags{}, map[string]bool{})
	if got != envPath {
		t.Errorf("got %q, want %q", got, envPath)
	}
}

func TestResolveConfigPath_FlagOverridesEnv(t *testing.T) {
	orig := configPath
	configPath = "/default/config.yaml"
	defer func() { configPath = orig }()

	dir := t.TempDir()
	envPath := filepath.Join(dir, "env.yaml")
	flagPath := filepath.Join(dir, "flag.yaml")
	t.Setenv("GO_WAYBAR_SIDECAR_CONFIG", envPath)

	f := cliFlags{config: flagPath}
	set := map[string]bool{"config": true}
	got := resolveConfigPath(f, set)
	if got != flagPath {
		t.Errorf("got %q, want %q", got, flagPath)
	}
}

// resolveSettings tests

func ptr[T any](v T) *T { return &v }

func TestResolveSettings_Defaults(t *testing.T) {
	os.Unsetenv("GO_WAYBAR_SIDECAR_LOG_LEVEL")
	os.Unsetenv("GO_WAYBAR_SIDECAR_LOG_OUTPUT")
	os.Unsetenv("GO_WAYBAR_SIDECAR_NOTIFY_LEVEL")
	os.Unsetenv("GO_WAYBAR_SIDECAR_NO_GENERATE")
	os.Unsetenv("GO_WAYBAR_SIDECAR_DAEMON")

	r := resolveSettings(cliFlags{}, map[string]bool{}, sidecarSettings{})
	if r.logLevel != defaultLogLevel {
		t.Errorf("logLevel: got %q, want %q", r.logLevel, defaultLogLevel)
	}
	if r.logOutput != defaultLogOutput {
		t.Errorf("logOutput: got %q, want %q", r.logOutput, defaultLogOutput)
	}
	if r.notifyLevel != defaultNotifyLevel {
		t.Errorf("notifyLevel: got %q, want %q", r.notifyLevel, defaultNotifyLevel)
	}
	if r.noGenerate != defaultNoGenerate {
		t.Errorf("noGenerate: got %v, want %v", r.noGenerate, defaultNoGenerate)
	}
	if r.daemon != defaultDaemon {
		t.Errorf("daemon: got %v, want %v", r.daemon, defaultDaemon)
	}
}

func TestResolveSettings_FileOverridesDefaults(t *testing.T) {
	os.Unsetenv("GO_WAYBAR_SIDECAR_LOG_LEVEL")
	os.Unsetenv("GO_WAYBAR_SIDECAR_LOG_OUTPUT")
	os.Unsetenv("GO_WAYBAR_SIDECAR_NOTIFY_LEVEL")
	os.Unsetenv("GO_WAYBAR_SIDECAR_NO_GENERATE")
	os.Unsetenv("GO_WAYBAR_SIDECAR_DAEMON")

	s := sidecarSettings{
		LogLevel:    ptr("debug"),
		LogOutput:   ptr("/var/log/gws.log"),
		NotifyLevel: ptr("warn"),
		NoGenerate:  ptr(true),
		Daemon:      ptr(true),
	}
	r := resolveSettings(cliFlags{}, map[string]bool{}, s)
	if r.logLevel != "debug" {
		t.Errorf("logLevel: got %q, want debug", r.logLevel)
	}
	if r.logOutput != "/var/log/gws.log" {
		t.Errorf("logOutput: got %q", r.logOutput)
	}
	if r.notifyLevel != "warn" {
		t.Errorf("notifyLevel: got %q, want warn", r.notifyLevel)
	}
	if !r.noGenerate {
		t.Error("noGenerate: expected true")
	}
	if !r.daemon {
		t.Error("daemon: expected true")
	}
}

func TestResolveSettings_EnvOverridesFile(t *testing.T) {
	t.Setenv("GO_WAYBAR_SIDECAR_LOG_LEVEL", "error")
	t.Setenv("GO_WAYBAR_SIDECAR_LOG_OUTPUT", "stderr")
	t.Setenv("GO_WAYBAR_SIDECAR_NOTIFY_LEVEL", "off")
	t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", "1")
	t.Setenv("GO_WAYBAR_SIDECAR_DAEMON", "0")

	s := sidecarSettings{
		LogLevel:    ptr("debug"),
		LogOutput:   ptr("/var/log/gws.log"),
		NotifyLevel: ptr("warn"),
		NoGenerate:  ptr(false),
		Daemon:      ptr(true),
	}
	r := resolveSettings(cliFlags{}, map[string]bool{}, s)
	if r.logLevel != "error" {
		t.Errorf("logLevel: got %q, want error", r.logLevel)
	}
	if r.logOutput != "stderr" {
		t.Errorf("logOutput: got %q, want stderr", r.logOutput)
	}
	if r.notifyLevel != "off" {
		t.Errorf("notifyLevel: got %q, want off", r.notifyLevel)
	}
	if !r.noGenerate {
		t.Error("noGenerate: expected true (from env 1)")
	}
	if r.daemon {
		t.Error("daemon: expected false (from env 0)")
	}
}

func TestResolveSettings_FlagOverridesEnv(t *testing.T) {
	t.Setenv("GO_WAYBAR_SIDECAR_LOG_LEVEL", "warn")
	t.Setenv("GO_WAYBAR_SIDECAR_LOG_OUTPUT", "stderr")
	t.Setenv("GO_WAYBAR_SIDECAR_NOTIFY_LEVEL", "info")
	t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", "1")
	t.Setenv("GO_WAYBAR_SIDECAR_DAEMON", "1")

	f := cliFlags{
		logLevel:    "debug",
		logOutput:   "stdout",
		notifyLevel: "error",
		noGenerate:  false,
		daemon:      false,
	}
	set := map[string]bool{
		"log-level":    true,
		"log-output":   true,
		"notify-level": true,
		"no-generate":  true,
		"daemon":       true,
	}
	r := resolveSettings(f, set, sidecarSettings{})
	if r.logLevel != "debug" {
		t.Errorf("logLevel: got %q, want debug", r.logLevel)
	}
	if r.logOutput != "stdout" {
		t.Errorf("logOutput: got %q, want stdout", r.logOutput)
	}
	if r.notifyLevel != "error" {
		t.Errorf("notifyLevel: got %q, want error", r.notifyLevel)
	}
	if r.noGenerate {
		t.Error("noGenerate: expected false (flag wins over env)")
	}
	if r.daemon {
		t.Error("daemon: expected false (flag wins over env)")
	}
}

func TestResolveSettings_BoolEnv_ValidValues(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
		{"yes", true},
		{"no", false},
		{"YES", true},
		{"NO", false},
		{"Yes", true},
		{"No", false},
	}
	for _, tc := range cases {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", tc.val)
			os.Unsetenv("GO_WAYBAR_SIDECAR_DAEMON")
			r := resolveSettings(cliFlags{}, map[string]bool{}, sidecarSettings{})
			if r.noGenerate != tc.want {
				t.Errorf("GO_WAYBAR_SIDECAR_NO_GENERATE=%q: got %v, want %v", tc.val, r.noGenerate, tc.want)
			}
		})
	}
}

func TestResolveSettings_BoolEnv_InvalidIgnored(t *testing.T) {
	t.Setenv("GO_WAYBAR_SIDECAR_NO_GENERATE", "yes-please")
	os.Unsetenv("GO_WAYBAR_SIDECAR_DAEMON")
	// invalid value → falls back to default (false)
	s := sidecarSettings{NoGenerate: ptr(false)}
	r := resolveSettings(cliFlags{}, map[string]bool{}, s)
	if r.noGenerate {
		t.Error("invalid env bool should be ignored; expected noGenerate=false")
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
