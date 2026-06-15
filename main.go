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

func parseFlags() cliFlags {
	var f cliFlags
	flag.StringVar(&f.logLevel, "log-level", "info", "log level: debug, info, warn, error")
	flag.StringVar(&f.logOutput, "log-output", "stdout", "log destination: stdout, stderr, or a file path")
	flag.StringVar(&f.notifyLevel, "notify-level", "error", "notification threshold: off, debug, info, warn, error")
	flag.StringVar(&f.config, "config", "", "path to config.yaml (default: "+configPath+")")
	flag.BoolVar(&f.daemon, "daemon", false, "fork to background")
	flag.BoolVar(&f.noGenerate, "no-generate", false, "watch and reload waybar without generating config.jsonc/menus from config.yaml")

	flag.Usage = printUsage

	flag.Parse()
	return f
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

Config:
  Source:   %s
  Menu dir: %s
  JSON out: %s
  PID dir:  %s
`, configPath, configPath, configPath, menuDir, outJSON, pidDir)
}

func main() {
	flags := parseFlags()

	if flags.config != "" {
		configPath = resolvedPath(flags.config)
	}

	noGenerate = flags.noGenerate

	level, err := parseLogLevel(flags.logLevel)
	if err != nil {
		slog.Error("invalid log level", "value", flags.logLevel)
		os.Exit(1)
	}

	notifyLevel, notifyEnabled, err := parseNotifyLevel(flags.notifyLevel)
	if err != nil {
		slog.Error("invalid notify level", "value", flags.notifyLevel)
		os.Exit(1)
	}

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if flags.daemon {
		forkDetach()
		return
	}

	logger, err = buildLogger(level, flags.logOutput, notifyLevel, notifyEnabled)
	if err != nil {
		slog.Error("failed to open log output", "output", flags.logOutput, "err", err)
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
	savePID("waybar-sidecar", os.Getpid())

	var cfg Config
	if !noGenerate {
		ensureConfig()
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

	if !notifyAvailable() {
		logger := slog.New(textHandler)
		logger.Info("desktop notifications inactive", "reason", "notify-send not found")
		return logger, nil
	}

	notify := &notifyHandler{threshold: notifyLevel, appName: notifyAppName}
	return slog.New(&multiHandler{handlers: []slog.Handler{textHandler, notify}}), nil
}

func forkDetach() {
	args := make([]string, 0, len(os.Args))
	for _, a := range os.Args[1:] {
		if a != "--daemon" && a != "-daemon" {
			args = append(args, a)
		}
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

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
