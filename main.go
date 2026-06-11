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

func main() {
	levelFlag := flag.String("log-level", "info", "log level: debug, info, warn, error")
	outputFlag := flag.String("log-output", "stdout", "log destination: stdout, stderr, or a file path")
	notifyLevelFlag := flag.String("notify-level", "error", "notification threshold: off, debug, info, warn, error")
	daemonFlag := flag.Bool("daemon", false, "fork to background")

	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, `waybar-sidecar — generates waybar config from %s and keeps waybar running.

Usage:
  waybar-sidecar [flags]

Flags:
  --daemon              fork to background (stdio is closed; combine with --log-output to keep logs)
  --log-level <level>   log verbosity: debug, info, warn, error  (default: info)
  --log-output <dest>   log destination: stdout, stderr, or a file path  (default: stdout)
  --notify-level <lvl>  notification threshold: off, debug, info, warn, error  (default: error)
  --help                show this help

Config:
  Source:  %s
  XML out: %s
  JSON out: %s
  PID dir: %s
`, configPath, configPath, outXML, outJSON, pidDir)
	}

	flag.Parse()

	level, err := parseLogLevel(*levelFlag)
	if err != nil {
		slog.Error("invalid log level", "value", *levelFlag)
		os.Exit(1)
	}

	notifyLevel, notifyEnabled, err := parseNotifyLevel(*notifyLevelFlag)
	if err != nil {
		slog.Error("invalid notify level", "value", *notifyLevelFlag)
		os.Exit(1)
	}

	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	if *daemonFlag {
		forkDetach()
		return
	}

	logger, err = buildLogger(level, *outputFlag, notifyLevel, notifyEnabled)
	if err != nil {
		slog.Error("failed to open log output", "output", *outputFlag, "err", err)
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
		"out_xml", outXML,
		"out_json", outJSON,
	)

	ensureSingleInstance()
	savePID("waybar-sidecar", os.Getpid())
	ensureConfig()
	cfg := loadConfig()
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
