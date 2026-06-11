package main

import (
	_ "embed"
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
	configPath = filepath.Join(waybarDir, "go-waybar-sidecar", "config.yaml")

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

func loadConfig() Config {
	logger.Debug("loading config", "path", configPath)

	b, err := os.ReadFile(configPath)
	if err != nil {
		logger.Error("failed to read config", "path", configPath, "err", err)
		os.Exit(1)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		logger.Error("failed to parse config", "path", configPath, "err", err)
		os.Exit(1)
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
			logger.Error("failed to parse menu_definition", "module", name, "err", err)
			os.Exit(1)
		}
		var items []MenuItem
		if err := yaml.Unmarshal(mb, &items); err != nil {
			logger.Error("failed to parse menu_definition", "module", name, "err", err)
			os.Exit(1)
		}
		menus[name] = items
	}

	cfg := Config{
		Waybar: raw.Waybar,
		Menus:  menus,
	}

	logger.Debug("config loaded",
		"waybar_keys", len(cfg.Waybar),
		"menus", len(cfg.Menus),
	)
	return cfg
}
