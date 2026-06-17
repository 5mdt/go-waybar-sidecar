# GWS-0020. Persist default sidecar settings to config file

**Tags:** #config

## Description

On startup, any `sidecar:` parameter absent from `config.yaml` is written back
to the file using its current built-in default. This pins each user's settings:
if a built-in default changes in a future release, existing installs retain the
behaviour they already had.

Rules:
- Only **built-in defaults** are written, never the effective resolved value
  (which may include transient env-var or flag overrides).
- Keys the user already set are **never overwritten**, even if their value
  differs from the built-in default.
- Comments, the `waybar:` section, and key order are preserved — the YAML AST
  (`yaml.Node`) is used instead of a map round-trip.
- A `.bak` backup of the previous file is created **only** when at least one
  key is actually added. Repeated startups with a complete file produce no I/O.

## Implementation

`config.go`:
- `sidecarDefaults` — ordered descriptor list of the five settings (key name,
  YAML type tag, built-in default value string).
- `ensureSettings(path string) error` — called once at startup after
  `ensureConfig()`:
  1. Read the file; return nil (no-op) if missing or unparseable.
  2. Unmarshal into `yaml.Node` (document node). Locate the root
     `MappingNode`; skip if malformed.
  3. Find or create the `sidecar` mapping in the root map.
  4. For each descriptor not already present in the sidecar map, append a key
     scalar + value scalar.
  5. If nothing was added, return nil (idempotent).
  6. If keys were added: call `backupFile(path)` (reuses `generator.go:161`),
     then write the re-encoded YAML (`yaml.NewEncoder` with `SetIndent(2)`) to
     `path`.

`main.go` (`run()`):
- Call `ensureSettings(configPath)` immediately after `ensureConfig()` (inside
  the `if !noGenerate` block). Errors are logged as warnings; the app continues.
  The write happens before `go watch(&cfg)` starts, preventing a self-trigger.

## Configuration

No new user-facing settings. The effect is transparent: `config.yaml` gains
any missing `sidecar:` keys with their defaults on first run after an upgrade.
The `.bak` extension is ignored by `isWatchedFile` (`watcher.go:108`), so no
spurious reload is triggered.

## Testing

### Human Testing

- `config.yaml` with only `waybar:` (no `sidecar:`) → after startup, a
  `sidecar:` block with all five defaults appears; `waybar:` content is
  byte-for-byte unchanged; a `config.yaml.bak` holds the original.
- `sidecar.log-level: debug`, other four keys absent → restart: four keys
  added with defaults, `log-level: debug` untouched, `.bak` created.
- Restart again with a now-complete file → no `.bak` churn, file unchanged.

### Unit Testing

- All sidecar keys absent → after call, file has all five with built-in
  defaults; `waybar:` section still present.
- Partial section (`log-level: debug` only) → other four added; `log-level`
  stays `debug`.
- All keys present → no change; `config.yaml.bak` is **not** created.
- User non-default value (e.g. `daemon: true`) → preserved after fill.
- Comment in sidecar section survives the rewrite.
- Backup created on change → `.bak` exists with original content.
- Missing file → no error, no file created.

### Integration Testing

- Launch with `HOME` pointing at an empty temp dir (after `ensureConfig` writes
  the embedded default): `ensureSettings` finds all five keys present → no
  `.bak` produced.
- Launch with a hand-crafted `config.yaml` that omits `notify-level`: after
  startup `notify-level: error` appears in the file.

## Status

Implemented.
