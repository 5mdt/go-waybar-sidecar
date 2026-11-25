package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

var logger *slog.Logger

type cliFlags struct {
	logLevel    string
	logOutput   string
	notifyLevel string
	config      string
	daemon      bool
	noGenerate  bool
}

func parseFlags() (cliFlags, map[string]bool) {
	var f cliFlags
	flag.StringVar(&f.logLevel, "log-level", defaultLogLevel, "log level: debug, info, warn, error")
	flag.StringVar(&f.logOutput, "log-output", defaultLogOutput, "log destination: stdout, stderr, or a file path")
	flag.StringVar(&f.notifyLevel, "notify-level", defaultNotifyLevel, "notification threshold: off, debug, info, warn, error")
	flag.StringVar(&f.config, "config", "", "path to config.yaml (default: "+configPath+")")
	flag.BoolVar(&f.daemon, "daemon", defaultDaemon, "fork to background")
	flag.BoolVar(&f.noGenerate, "no-generate", defaultNoGenerate, "watch and reload waybar without generating config.jsonc/menus from config.yaml")

	flag.Usage = printUsage

	flag.Parse()

	set := make(map[string]bool)
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	return f, set
}

func printUsage() {
	fmt.Fprintf(os.Stdout, `waybar-sidecar — generates waybar config from %s and keeps waybar running.

Usage:
  waybar-sidecar [flags]

Flags:
  --daemon              fork to background (stdio is closed; combine with --log-output to keep logs)
  --log-level <level>   log verbosity: debug, info, warn, error  (default: info)
  --log-output <dest>   log destination: stdout, stderr, or a file path  (default: stdout)
  --notify-level <lvl>  notification threshold: off, debug, info, warn, error  (default: error)
  --config <path>       path to config.yaml  (default: %s)
  --no-generate         watch and reload waybar without generating config.jsonc/menus from config.yaml
  --help                show this help

Environment (override config file; flags take precedence over env):
  GO_WAYBAR_SIDECAR_LOG_LEVEL      overrides --log-level
  GO_WAYBAR_SIDECAR_LOG_OUTPUT     overrides --log-output
  GO_WAYBAR_SIDECAR_NOTIFY_LEVEL   overrides --notify-level
  GO_WAYBAR_SIDECAR_NO_GENERATE    overrides --no-generate  (true/false/yes/no/1/0)
  GO_WAYBAR_SIDECAR_DAEMON         overrides --daemon       (true/false/yes/no/1/0)
  GO_WAYBAR_SIDECAR_CONFIG         overrides --config path

Config file settings (sidecar: section in config.yaml, lowest override priority):
  sidecar:
    log-level: info
    log-output: stdout
    notify-level: error
    no-generate: false
    daemon: false

Periodic jobs (jobs: section in config.yaml):
  jobs:
    - name: example
      every: 5m              # or: schedule: "*/5 * * * *"
      command: "echo hello"
      output: ~/.cache/waybar/example  # optional; stdout written here

Config:
  Source:   %s
  Menu dir: %s
  JSON out: %s
  PID dir:  %s
`, configPath, configPath, configPath, menuDir, outJSON, pidDir)
}

func main() {
	flags, set := parseFlags()

	configPath = resolveConfigPath(flags, set)

	sidecar := loadSettings(configPath)
	resolved := resolveSettings(flags, set, sidecar)

	noGenerate = resolved.noGenerate

	level, err := parseLogLevel(resolved.logLevel)
	if err != nil {
		slog.Error("invalid log level", "value", resolved.logLevel)
		os.Exit(1)
	}

	notifyLevel, notifyEnabled, err := parseNotifyLevel(resolved.notifyLevel)
	if err != nil {
		slog.Error("invalid notify level", "value", resolved.notifyLevel)
		os.Exit(1)
	}

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if resolved.daemon && os.Getenv("GO_WAYBAR_SIDECAR_FORKED") == "" {
		forkDetach()
		return
	}

	logger, err = buildLogger(level, resolved.logOutput, notifyLevel, notifyEnabled)
	if err != nil {
		slog.Error("failed to open log output", "output", resolved.logOutput, "err", err)
		os.Exit(1)
	}

	run()
}

func run() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range sigs {
			switch sig {
			case syscall.SIGHUP:
				logger.Debug("SIGHUP received (ignored)")
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("shutting down", "signal", sig)
				waybar.stop()
				os.Exit(0)
			}
		}
	}()

	logger.Info("starting waybar sidecar",
		"config", configPath,
		"menu_dir", menuDir,
		"out_json", outJSON,
	)

	ensureSingleInstance()
	sweepStrayWaybars()

	var cfg Config
	if !noGenerate {
		ensureConfig()
		if err := ensureSettings(configPath); err != nil {
			logger.Warn("failed to fill default settings", "err", err)
		}
		var err error
		cfg, err = loadConfig()
		if err != nil {
			logger.Error("failed to load config, falling back to default", "err", err)
			cfg, err = parseConfig(defaultConfig)
			if err != nil {
				logger.Error("failed to parse default config", "err", err)
				os.Exit(1)
			}
		}
	}
	generate(cfg)
	jobs.restart(cfg.Jobs)
	go watch(&cfg)

	select {}
}

func buildLogger(level slog.Level, output string, notifyLevel slog.Level, notifyEnabled bool) (*slog.Logger, error) {
	var w *os.File
	switch output {
	case "stdout":
		w = os.Stdout
	case "stderr":
		w = os.Stderr
	default:
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return nil, fmt.Errorf("create log dir: %w", err)
		}
		f, err := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		w = f
	}

	textHandler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})

	if !notifyEnabled {
		return slog.New(textHandler), nil
	}

	n, ok := newNotifier(notifyAppName)
	if !ok {
		logger := slog.New(textHandler)
		logger.Info("desktop notifications inactive", "reason", "notification bus unavailable")
		return logger, nil
	}
	sendNotification = n.send

	notify := &notifyHandler{threshold: notifyLevel}
	return slog.New(&multiHandler{handlers: []slog.Handler{textHandler, notify}}), nil
}

func forkDetach() {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = append(os.Environ(), "GO_WAYBAR_SIDECAR_FORKED=1")

	if err := cmd.Start(); err != nil {
		logger.Error("failed to fork", "err", err)
		os.Exit(1)
	}

	logger.Info("forked to background", "pid", cmd.Process.Pid)
}

func parseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	return level, level.UnmarshalText([]byte(s))
}
