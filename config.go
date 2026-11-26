package main

import (
	"bytes"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var defaultConfig []byte

type MenuItem struct {
	Name     string     `yaml:"name"`
	Action   string     `yaml:"action"`
	Children []MenuItem `yaml:"children"`
	// Icon is an icon-theme name, or a file path if it contains "/" (GWS-0032).
	Icon string `yaml:"icon"`
	// Include, when set, replaces this item in place with a generated list of
	// items (see expandIncludes). Name/Action/Children are ignored.
	Include string `yaml:"include"`
	// Mode selects category placement for applications:categories:
	// "all" (default) or "first" (GWS-0031). Ignored by other aliases.
	Mode string `yaml:"mode"`
	// Categories are the .desktop Categories= values. Set only by
	// scanApplications, never by user config.
	Categories []string `yaml:"-"`
	// Bold renders the item's label with Pango markup instead of the plain
	// GtkMenuItem label (see addNode). Set only by generators such as
	// buildPowerProfileItems, never by user config.
	Bold bool `yaml:"-"`
}

// Job defines a periodic command run by the jobs runner (GWS-0025).
type Job struct {
	Name     string `yaml:"name"`
	Every    string `yaml:"every"`
	Schedule string `yaml:"schedule"`
	Command  string `yaml:"command"`
	Output   string `yaml:"output"`
}

type Config struct {
	Waybar map[string]any
	Menus  map[string][]MenuItem
	Jobs   []Job
}

type rawConfig struct {
	Waybar  map[string]any    `yaml:"waybar"`
	Sidecar map[string]string `yaml:"sidecar"`
	Jobs    []Job             `yaml:"jobs"`
}

// resolvedSettings is the merged result of defaults < config file < env < flags.
type resolvedSettings struct {
	logLevel    string
	logOutput   string
	notifyLevel string
	noGenerate  bool
	daemon      bool
}

const (
	defaultLogLevel    = "info"
	defaultLogOutput   = "stdout"
	defaultNotifyLevel = "error"
	defaultNoGenerate  = false
	defaultDaemon      = false
)

// setting describes one sidecar setting: its sidecar: key (also the flag
// name), env var, built-in default (as text), and flag usage. Flags, env, the
// config file, --help, and ensureSettings are all driven by this table.
type setting struct {
	key, env, def, usage string
	isBool               bool
}

const configEnv = "GO_WAYBAR_SIDECAR_CONFIG"

var settings = []setting{
	{"log-level", "GO_WAYBAR_SIDECAR_LOG_LEVEL", defaultLogLevel, "log level: debug, info, warn, error", false},
	{"log-output", "GO_WAYBAR_SIDECAR_LOG_OUTPUT", defaultLogOutput, "log destination: stdout, stderr, or a file path", false},
	{"notify-level", "GO_WAYBAR_SIDECAR_NOTIFY_LEVEL", defaultNotifyLevel, "notification threshold: off, debug, info, warn, error", false},
	{"no-generate", "GO_WAYBAR_SIDECAR_NO_GENERATE", strconv.FormatBool(defaultNoGenerate), "watch and reload waybar without generating config.jsonc/menus from config.yaml", true},
	{"daemon", "GO_WAYBAR_SIDECAR_DAEMON", strconv.FormatBool(defaultDaemon), "fork to background (stdio is closed; combine with --log-output to keep logs)", true},
}

// loadSettings reads the sidecar: section from path. A missing file or an
// absent section yields an empty map; unreadable or malformed YAML is an error
// (the caller falls back to env vars and defaults).
func loadSettings(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var raw rawConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return raw.Sidecar, nil
}

// passedFlags returns the explicitly set flags of fs and their values.
func passedFlags(fs *flag.FlagSet) map[string]string {
	set := map[string]string{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })
	return set
}

// resolveConfigPath applies: default configPath < GO_WAYBAR_SIDECAR_CONFIG < --config flag.
func resolveConfigPath(fs *flag.FlagSet) string {
	path := configPath
	if v := os.Getenv(configEnv); v != "" {
		path = v
	}
	if v, ok := passedFlags(fs)["config"]; ok {
		path = v
	}
	return resolvedPath(path)
}

// parseBoolEnv parses a boolean env-var value, extending strconv.ParseBool
// with "yes" (true) and "no" (false), case-insensitive.
func parseBoolEnv(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "yes":
		return true, true
	case "no":
		return false, true
	}
	b, err := strconv.ParseBool(v)
	return b, err == nil
}

// resolveSettings applies, per setting: defaults < config file < env vars <
// CLI flags (only those actually passed). An invalid boolean from any source
// is an error.
func resolveSettings(fs *flag.FlagSet, file map[string]string) (resolvedSettings, error) {
	passed := passedFlags(fs)
	vals := make(map[string]string, len(settings))
	for _, s := range settings {
		v := s.def
		if f, ok := file[s.key]; ok {
			v = f
		}
		if e, ok := os.LookupEnv(s.env); ok {
			v = e
		}
		if f, ok := passed[s.key]; ok {
			v = f
		}
		if s.isBool {
			b, ok := parseBoolEnv(v)
			if !ok {
				return resolvedSettings{}, fmt.Errorf("invalid boolean %q for %s", v, s.key)
			}
			v = strconv.FormatBool(b)
		}
		vals[s.key] = v
	}

	return resolvedSettings{
		logLevel:    vals["log-level"],
		logOutput:   vals["log-output"],
		notifyLevel: vals["notify-level"],
		noGenerate:  vals["no-generate"] == "true",
		daemon:      vals["daemon"] == "true",
	}, nil
}

// ensureSettings fills any absent sidecar: keys in path with their built-in
// defaults. Existing keys (including non-default user values) are never
// changed. The file is only written (and backed up to path+".bak") when at
// least one key is actually added. Comments and the waybar: section are
// preserved via yaml.Node AST manipulation. Returns nil on missing/unreadable
// files - callers fall back to the live defaults instead.
func ensureSettings(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil // missing file is not an error here
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil // unparseable; leave file untouched
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}

	// Find or create the sidecar mapping in the root map.
	var sidecarMap *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "sidecar" {
			val := root.Content[i+1]
			if val.Kind != yaml.MappingNode {
				// Replace null / scalar with an empty mapping.
				val.Kind = yaml.MappingNode
				val.Tag = "!!map"
				val.Value = ""
				val.Content = nil
			}
			sidecarMap = val
			break
		}
	}
	if sidecarMap == nil {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "sidecar"}
		sidecarMap = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content, keyNode, sidecarMap)
	}

	// Collect keys already present in the sidecar mapping.
	existing := make(map[string]bool, len(sidecarMap.Content)/2)
	for i := 0; i+1 < len(sidecarMap.Content); i += 2 {
		existing[sidecarMap.Content[i].Value] = true
	}

	// Append missing keys with their built-in defaults.
	added := 0
	for _, d := range settings {
		if existing[d.key] {
			continue
		}
		tag := "!!str"
		if d.isBool {
			tag = "!!bool"
		}
		sidecarMap.Content = append(sidecarMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: d.key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: d.def},
		)
		added++
	}

	if added == 0 {
		return nil // nothing to do
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close encoder: %w", err)
	}

	if err := backupFile(path); err != nil {
		return fmt.Errorf("backup config before settings fill: %w", err)
	}

	if err := writeFileAtomic(path, buf.Bytes()); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

var (
	waybarDir  = resolvedPath(filepath.Join(os.Getenv("HOME"), ".config/waybar"))
	gwsDir     = resolvedPath(filepath.Join(os.Getenv("HOME"), ".config/go-waybar-sidecar"))
	configPath = filepath.Join(gwsDir, "config.yaml")

	menuDir = filepath.Join(waybarDir, "gtk3-menus")
	outJSON = filepath.Join(waybarDir, "config.jsonc")
)

func ensureConfig() error {
	if err := os.MkdirAll(waybarDir, 0o755); err != nil {
		return fmt.Errorf("create waybar dir %s: %w", waybarDir, err)
	}

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		return fmt.Errorf("create menu output dir %s: %w", menuDir, err)
	}

	if _, err := os.Stat(configPath); err == nil {
		logger.Debug("config already exists", "path", configPath)
		return nil
	}

	logger.Info("config not found, writing default", "path", configPath)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", filepath.Dir(configPath), err)
	}

	if err := os.WriteFile(configPath, defaultConfig, 0o644); err != nil {
		return fmt.Errorf("write default config %s: %w", configPath, err)
	}
	return nil
}

func loadConfig() (Config, error) {
	logger.Debug("loading config", "path", configPath)

	b, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", configPath, err)
	}

	cfg, err := parseConfig(b)
	if err != nil {
		return Config{}, err
	}

	for name, items := range cfg.Menus {
		cfg.Menus[name] = expandIncludes(items)
	}

	logger.Debug("config loaded",
		"waybar_keys", len(cfg.Waybar),
		"menus", len(cfg.Menus),
	)
	return cfg, nil
}

// parseConfig decodes raw config.yaml bytes into a Config, extracting each
// module's menu_definition (if any) into cfg.Menus.
func parseConfig(b []byte) (Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	menus := map[string][]MenuItem{}
	for name, modVal := range raw.Waybar {
		mod, ok := modVal.(map[string]any)
		if !ok {
			continue
		}
		md, ok := mod["menu_definition"]
		if !ok {
			continue
		}

		mb, err := yaml.Marshal(md)
		if err != nil {
			return Config{}, fmt.Errorf("parse menu_definition for %s: %w", name, err)
		}
		var items []MenuItem
		if err := yaml.Unmarshal(mb, &items); err != nil {
			return Config{}, fmt.Errorf("parse menu_definition for %s: %w", name, err)
		}
		menus[name] = items
	}

	return Config{
		Waybar: raw.Waybar,
		Menus:  menus,
		Jobs:   raw.Jobs,
	}, nil
}

// expandIncludes recursively replaces each item with a non-empty Include by
// the resolved alias's items, splicing them in place of the item (not
// wrapped in a submenu). Unknown aliases are dropped with a warning. See
// GWS-0028.
func expandIncludes(items []MenuItem) []MenuItem {
	expanded := make([]MenuItem, 0, len(items))
	for _, item := range items {
		if item.Include != "" {
			resolved, ok := resolveInclude(item)
			if !ok {
				logger.Warn("unknown include alias, dropping item", "include", item.Include)
				continue
			}
			expanded = append(expanded, resolved...)
			continue
		}

		item.Children = expandIncludes(item.Children)
		expanded = append(expanded, item)
	}
	return expanded
}

// resolveInclude resolves a single include alias to its generated items.
func resolveInclude(item MenuItem) ([]MenuItem, bool) {
	switch item.Include {
	case "applications:plain":
		return scanApplications(), true
	case "applications:categories":
		return categorizeApps(scanApplications(), item.Mode), true
	case "power-profiles:plain":
		return scanPowerProfiles(), true
	case "session-actions:plain":
		return scanSessionActions(), true
	default:
		return nil, false
	}
}
