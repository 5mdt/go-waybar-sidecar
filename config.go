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
	Waybar    map[string]any
	StartMenu []MenuItem
}

type rawConfig struct {
	Waybar map[string]any `yaml:"waybar"`
}

var (
	waybarDir  = resolvedPath(filepath.Join(os.Getenv("HOME"), ".config/waybar"))
	configPath = filepath.Join(waybarDir, "config.yaml")

	outXML  = filepath.Join(waybarDir, "start.xml")
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

	var menuRaw struct {
		Waybar struct {
			Start struct {
				MenuDefinition []MenuItem `yaml:"menu_definition"`
			} `yaml:"custom/Start"`
		} `yaml:"waybar"`
	}
	if err := yaml.Unmarshal(b, &menuRaw); err != nil {
		logger.Error("failed to parse config", "path", configPath, "err", err)
		os.Exit(1)
	}

	cfg := Config{
		Waybar:    raw.Waybar,
		StartMenu: menuRaw.Waybar.Start.MenuDefinition,
	}

	logger.Debug("config loaded",
		"waybar_keys", len(cfg.Waybar),
		"menu_items", len(cfg.StartMenu),
	)
	return cfg
}
