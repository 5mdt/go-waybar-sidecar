# Go, Waybar Sidecar!

Generate configs for Waybar with menus, monitors waybar state. Suspiciously, code has an LLM aftertaste.

## Features

- Monitors configuration and style files in config folder and start/reload Waybar. Waybar has no live reloading of config [yet](https://github.com/Alexays/Waybar/issues/961). Same behaviour can be got with simple shell script like [this](https://github.com/Alexays/Waybar/issues/961#issuecomment-2661418273).
- Small resource usage: ~0.0% CPU, ~10 MB RAM. It's a bit worse than bash + inotify (RSS is for really used RAM):

  ```table
  USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
  nett00n     1823  0.0  0.0   6944  3732 tty2     S+   Jun14   0:00 inotifywait -m -r -e modify,create,delete /home/nett00n/.config/waybar/
  nett00n   581250  0.0  0.0 1784092 10600 ?       Sl   10:18   0:00 go-waybar-sidecar
  ```
- Generates configuration from YAML file. I really *hate* json files as human-writable configuration format :)
- Generates gtk3-menu XML files from YAML with actions and submenus. This unifies Waybar configuration in one place.
- Generated menu entries: installed applications (plain or by category, with icons), power profiles, session actions (lock, suspend, reboot, ...).
- Supervises Waybar: restarts it if it dies, stops it on exit, takes over from an older sidecar instance.
- Cron-like jobs (`jobs:` in `config.yaml`) that run shell commands and write their output to files, e.g. for `custom/exec` modules.
- Settings via `sidecar:` in `config.yaml`, environment variables or flags.
- Optional clickable desktop notifications (mako, dunst, swaync, etc).
- Config generation is optional: `--no-generate` only watches and reloads Waybar.

[Feature Requirements Document](./docs/FRD.md) is a source of features truth. See also the [changelog](./docs/CHANGELOG.md).

## Installation

Requires Go and `waybar`. Installs to `~/.local/bin`:

```sh
make install
```

## Usage

```sh
go-waybar-sidecar --help
```

## License

[GPLv3](./LICENSE.md)

## Authors

5mdt team (sorted alphabetically):

- [@nett00n](https://github.com/nett00n)
