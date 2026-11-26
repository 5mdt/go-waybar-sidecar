# GWS-0004. Default config bootstrap

**Tags:** #config #config-generation

## Description

On startup, if `configPath` (default `~/.config/go-waybar-sidecar/config.yaml`, see GWS-0014) does not exist, the sidecar writes out a built-in default config so it has something to render.

## Implementation

`src/config.go`:
- `defaultConfig` is the contents of `src/config.yaml`, embedded into the binary at build time via `//go:embed config.yaml`.
- `ensureConfig()`, called from `run()` before `loadConfig()`:
  - If `configPath` exists, logs at debug and returns.
  - Otherwise logs at info, creates `filepath.Dir(configPath)` (`0o755`), and writes `defaultConfig` to `configPath` (`0o644`).
  - Any error creating the directory or writing the file is returned; `run()` logs it at error and exits the process (`os.Exit(1)`).

## Configuration

- `configPath` = `~/.config/go-waybar-sidecar/config.yaml` by default (via `gwsDir`, resolved through symlinks by `resolvedPath()`), or the path given via `--config` (see GWS-0014).
- The default content is the repo's `config.yaml`: a `waybar` section with `custom/Start`, `clock`, and `wlr/taskbar` modules, plus a default `menu_definition` (Apps → Terminal/Browser, Power → Shutdown/Reboot).

## Testing

### Human Testing

- Remove `configPath` (back it up first), start the sidecar, and confirm the file is recreated with the default content and an info log "config not found, writing default" is emitted.
- Start the sidecar again with the file present and confirm it logs "config already exists" at debug and does not overwrite the file.

### Unit Testing

- `ensureConfig()` with `configPath` pointing at a non-existent file in a temp dir: assert the file is created and its contents equal the embedded bytes.
- `ensureConfig()` with `configPath` pointing at an existing file: assert the file's contents are unchanged after the call.
- `ensureConfig()` with a `configPath` that cannot be created (parent is a regular file): returns an error.

### Integration Testing

- Launch the binary with `HOME` pointed at an empty temp directory: assert `~/.config/go-waybar-sidecar/config.yaml` is created and `generate()` subsequently succeeds using the default config.

## Status

Implemented.
