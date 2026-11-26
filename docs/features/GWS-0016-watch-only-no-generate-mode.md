# GWS-0016. Watch-only mode (--no-generate)

**Tags:** #config-generation #reload #cli

## Description

Some users hand-maintain their own `~/.config/waybar/config.jsonc` (and any GTK menu XML) and only want the sidecar's watcher + waybar reload/cold-start behavior, without it ever reading `config.yaml` or writing generated output. The `--no-generate` flag opts out of the yaml-to-output generation step (GWS-0001/GWS-0002/GWS-0003/GWS-0004/GWS-0015) while keeping live reload (GWS-0005/GWS-0006/GWS-0007) intact.

## Implementation

`generator.go`:
- Package var `noGenerate bool` (default `false`).
- Package var `doReload = reloadWaybar`, an indirection so tests can stub reload behavior.
- `generate(cfg Config)`:
  - When `noGenerate` is `true`, it logs that generation was skipped and calls `doReload()` directly - no XML/JSON is written.
  - Otherwise it calls `writeOutputs(cfg)` (`buildXML`/`writeJSON`) and then `doReload()`.

`main.go`:
- Flag `--no-generate` (bool, default `false`).
- After `flag.Parse()`, `noGenerate = *noGenerateFlag`.
- `flag.Usage` documents the flag.
- `run()`: when `noGenerate` is set, skips `ensureConfig()` and `loadConfig()` entirely (an empty `Config{}` is passed to `generate` instead) - a missing or invalid `config.yaml` is irrelevant in this mode.

`watcher.go`:
- `reload()` only calls `loadConfig()` when `!noGenerate`; it always calls `generate(cfg)`, which writes outputs only when `!noGenerate` and always reloads waybar once.

## Configuration

- `--no-generate`: when set, the sidecar watches `configPath` and `waybarDir` and reloads/cold-starts waybar on changes, but never reads `config.yaml` or writes `config.jsonc`/menu XML.

## Testing

### Human Testing

- Run `go-waybar-sidecar --no-generate --log-level debug`. Confirm waybar is reloaded/cold-started on startup, a "generation skipped" debug line is logged, and `~/.config/waybar/config.jsonc` / `gtk3-menus/*.xml` mtimes are unchanged.
- Touch `~/.config/waybar/config.jsonc`: confirm a reload fires without any output files being regenerated.
- Run without `--no-generate`: confirm output generation and reload behave as before.

### Unit Testing

- `generate(cfg)` with `noGenerate = true`: no files are written under `menuDir`/`outJSON`, and `doReload` is invoked exactly once.
- `generate(cfg)` with `noGenerate = false`: output files are written (as before) and `doReload` is invoked exactly once.

### Integration Testing

- None.

## Status

Implemented.
