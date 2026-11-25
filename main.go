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
	"time"
)

var logger *slog.Logger

// reloadDebounce is how long file/profile events settle before one reload.
const reloadDebounce = 200 * time.Millisecond

// newFlagSet registers one flag per settings row plus --config.
func newFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	for _, s := range settings {
		if s.isBool {
			fs.Bool(s.key, false, s.usage)
		} else {
			fs.String(s.key, s.def, s.usage)
		}
	}
	fs.String("config", "", "path to config.yaml (default: "+configPath+")")
	fs.Usage = func() { printUsage(fs) }
	return fs
}

func printUsage(fs *flag.FlagSet) {
	out := os.Stdout
	fmt.Fprintf(out, "waybar-sidecar — generates waybar config from %s and keeps waybar running.\n\nUsage:\n  waybar-sidecar [flags]\n\nFlags:\n", configPath)
	fs.SetOutput(out)
	fs.PrintDefaults()

	fmt.Fprintln(out, "\nEnvironment (override config file; flags take precedence over env;")
	fmt.Fprintln(out, "booleans accept true/false/yes/no/1/0):")
	for _, s := range settings {
		fmt.Fprintf(out, "  %-32s overrides --%s\n", s.env, s.key)
	}
	fmt.Fprintf(out, "  %-32s overrides --config path\n", configEnv)

	fmt.Fprintln(out, "\nConfig file settings (sidecar: section in config.yaml, lowest override priority):\n  sidecar:")
	for _, s := range settings {
		fmt.Fprintf(out, "    %s: %s\n", s.key, s.def)
	}

	fmt.Fprintf(out, `
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
`, configPath, menuDir, outJSON, pidDir)
}

func main() {
	fs := newFlagSet()
	_ = fs.Parse(os.Args[1:]) // ExitOnError

	configPath = resolveConfigPath(fs)

	file, err := loadSettings(configPath)
	if err != nil {
		slog.Warn("ignoring sidecar settings in config file", "err", err)
	}
	resolved, err := resolveSettings(fs, file)
	if err != nil {
		slog.Error("invalid setting", "err", err)
		os.Exit(1)
	}

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
		if err := forkDetach(); err != nil {
			logger.Error("failed to fork", "err", err)
			os.Exit(1)
		}
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
		if err := ensureConfig(); err != nil {
			logger.Error("failed to prepare config", "err", err)
			os.Exit(1)
		}
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

	reloads := make(chan struct{}, 1)
	trigger := debounce(reloadDebounce, func() {
		select {
		case reloads <- struct{}{}:
		default:
		}
	})
	go watchPowerProfiles(trigger)
	go func() {
		if err := watch(trigger); err != nil {
			logger.Error("watcher failed", "err", err)
			os.Exit(1)
		}
	}()

	for range reloads {
		cfg = reload(cfg)
	}
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

	q := make(chan notification, notifyQueueSize)
	go func() {
		for m := range q {
			_ = n.send(m)
		}
	}()

	notify := &notifyHandler{threshold: notifyLevel, queue: q}
	return slog.New(&multiHandler{handlers: []slog.Handler{textHandler, notify}}), nil
}

func forkDetach() error {
	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = append(os.Environ(), "GO_WAYBAR_SIDECAR_FORKED=1")

	if err := cmd.Start(); err != nil {
		return err
	}

	logger.Info("forked to background", "pid", cmd.Process.Pid)
	return nil
}

func parseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	return level, level.UnmarshalText([]byte(s))
}
