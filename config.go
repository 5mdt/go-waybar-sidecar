package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed config.yaml
var defaultConfig []byte

type MenuItem struct {
	Name     string     `yaml:"name"`
	Action   string     `yaml:"action"`
	Children []MenuItem `yaml:"children"`
}

type Config struct {
	Waybar map[string]any
	Menus  map[string][]MenuItem
}

type rawConfig struct {
	Waybar map[string]any `yaml:"waybar"`
}

var (
	waybarDir  = resolvedPath(filepath.Join(os.Getenv("HOME"), ".config/waybar"))
	gwsDir     = filepath.Join(waybarDir, "go-waybar-sidecar")
	configPath = filepath.Join(gwsDir, "config.yaml")

	menuDir = filepath.Join(waybarDir, "gtk3-menus")
	outJSON = filepath.Join(waybarDir, "config.jsonc")
)

func resolvedPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(r, filepath.Base(p))
	}
	return p
}

func ensureConfig() {
	if err := os.MkdirAll(waybarDir, 0o755); err != nil {
		logger.Error("failed to create waybar dir", "path", waybarDir, "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(menuDir, 0o755); err != nil {
		logger.Error("failed to create menu output dir", "path", menuDir, "err", err)
		os.Exit(1)
	}

	if _, err := os.Stat(configPath); err == nil {
		logger.Debug("config already exists", "path", configPath)
		return
	}

	logger.Info("config not found, writing default", "path", configPath)

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		logger.Error("failed to create config dir", "path", filepath.Dir(configPath), "err", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, defaultConfig, 0o644); err != nil {
		logger.Error("failed to write default config", "path", configPath, "err", err)
		os.Exit(1)
	}
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
	}, nil
}
