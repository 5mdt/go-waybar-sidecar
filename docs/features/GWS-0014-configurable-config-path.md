# GWS-0014. Configurable config path

**Tags:** #config #cli

## Description

The sidecar's source `config.yaml` lives in its own subdirectory,
`~/.config/waybar/go-waybar-sidecar/config.yaml`, separate from waybar's own
`config.jsonc` and per-menu XML files (GWS-0015). A `--config <path>` flag
overrides the location. Parent directories are created automatically if
missing.

## Implementation

`config.go`:
- `configPath` defaults to `filepath.Join(waybarDir, "go-waybar-sidecar",
  "config.yaml")`, i.e. `~/.config/waybar/go-waybar-sidecar/config.yaml`.
- `waybarDir` and `outJSON` (`config.jsonc`) are unchanged - they remain
  directly under `~/.config/waybar`, where waybar itself expects
  `config.jsonc`. `menuDir` (`~/.config/waybar/gtk3-menus`) holds one XML
  file per menu-bearing module (GWS-0015).
- `ensureConfig()` creates `waybarDir`, `menuDir`, and
  `filepath.Dir(configPath)` (`0o755` each) before writing the default
  config, so all required directories - including any custom `--config`
  directory - are created automatically on first run.

`main.go`:
- New flag `--config <path>` (default: empty). After `flag.Parse()`, if set,
  `configPath = resolvedPath(*configFlag)`, overriding the default before
  `run()` and `flag.Usage` reference it.
- `flag.Usage` shows the active default path.

## Configuration

- `--config <path>`: path to `config.yaml` (default:
  `~/.config/waybar/go-waybar-sidecar/config.yaml`).

## Testing

### Human Testing

- Fresh `HOME`, run with no flags: confirm
  `~/.config/waybar/go-waybar-sidecar/config.yaml` is created (directory
  included) with the default content, while `~/.config/waybar/config.jsonc`
  is written directly under `~/.config/waybar` and per-menu XML files are
  written under `~/.config/waybar/gtk3-menus/`.
- Run with `--config /tmp/custom/cfg.yaml` where `/tmp/custom` doesn't exist:
  confirm the directory and default file are created at that path and used.
- Edit the config at the custom path: confirm the watcher picks up the change
  and regenerates output.

### Unit Testing

- Default `configPath` equals `waybarDir/go-waybar-sidecar/config.yaml`.
- `ensureConfig()` with `configPath` under a non-existent nested directory:
  assert both the directory and file are created.

### Integration Testing

- Launch the binary with `HOME` pointed at an empty temp directory and no
  `--config`: assert `~/.config/waybar/go-waybar-sidecar/config.yaml` exists
  after startup and `~/.config/waybar/config.jsonc` and at least one file
  under `~/.config/waybar/gtk3-menus/` are generated.
- Launch with `--config <tmpdir>/sub/config.yaml`: assert the nested directory
  is created and the file is used as the config source.

## Status

Implemented.
