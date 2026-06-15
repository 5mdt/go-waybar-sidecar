# Go, Waybar Sidecar!

Generate configs for Waybar with menus, monitors waybar state. Suspiciously, code has an LLM aftertaste.

## Features

- Monitors configuration and style files in config folder and start/reload Waybar.
  Waybar has no live reloading of config [yet](https://github.com/Alexays/Waybar/issues/961) for 5 years now.
  Same bahaviour can be got with simple shell script like [this](https://github.com/Alexays/Waybar/issues/961#issuecomment-2661418273).
- Small resource usage: ~0.0% CPU, ~5 MB RAM. It's a bit worse than bash + inotify (RSS is for really used RAM):

  ```table
  USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
  nett00n     1823  0.0  0.0   6944  3732 tty2     S+   Jun14   0:00 inotifywait -m -r -e modify,create,delete /home/nett00n/.config/waybar/
  nett00n  1847779  0.0  0.0 1229224 4868 ?        Ssl  20:30   0:00 go-waybar-sidecar
  ```
- Generates configuration from YAML file. I really *hate* json files as human-writable configuration format :)
- Generates gtk3-menu XML files from YAML with actions and submenus. This unifies Waybar configuration in one place.
- Optional notifications with notify-send (mako, dunst, sway-nc, etc)
- Config generation is optional, you can use app only as

[Feature Requirements Document](./docs/FRD.md) is a source of features truth.

# Installation

TBF

# --help

`./main.go`
```Go
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
```

## License

[GPLv3](./LICENSE.md)

## Authors

5mdt team (sorted alphabetically):

- [@nett00n](https://github.com/nett00n)
