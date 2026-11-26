# GWS-0020. Persist default sidecar settings to config file

**Tags:** #config

## Description

On startup, any `sidecar:` parameter absent from `config.yaml` is written back to the file using its current built-in default. This pins each user's settings: if a built-in default changes in a future release, existing installs retain the behaviour they already had.

Rules:
- Only **built-in defaults** are written, never the effective resolved value (which may include transient env-var or flag overrides).
- Keys the user already set are **never overwritten**, even if their value differs from the built-in default.
- Comments, the `waybar:` section, and key order are preserved - the YAML AST (`yaml.Node`) is used instead of a map round-trip.
- A `.bak` backup of the previous file is created **only** when at least one key is actually added. Repeated startups with a complete file produce no I/O.

## Implementation

`src/config.go`:
- The `settings` table (GWS-0019) supplies each key, its built-in default, and its YAML tag (`!!bool` when `isBool`, else `!!str`).
- `ensureSettings(path string) error` - called once at startup after `ensureConfig()`:
  - Parses the file into a `yaml.Node`; a missing, unparseable or malformed file is a no-op.
  - Appends every table row missing from the `sidecar` mapping (created if absent).
  - Only if something was added: `backupFile(path)` (GWS-0018), then `writeFileAtomic` of the re-encoded YAML (indent 2).

`src/main.go` (`run()`):
- Calls `ensureSettings(configPath)` right after `ensureConfig()` (inside the `if !noGenerate` block). Errors are logged as warnings. It runs before the watcher starts, so the write cannot self-trigger.

## Configuration

No new user-facing settings. The effect is transparent: `config.yaml` gains any missing `sidecar:` keys with their defaults.

## Testing

### Human Testing

- `config.yaml` with only `waybar:` (no `sidecar:`) → after startup, a `sidecar:` block with all five defaults appears; `waybar:` content is byte-for-byte unchanged; a `config.yaml.bak` holds the original.
- `sidecar.log-level: debug`, other four keys absent → restart: four keys added with defaults, `log-level: debug` untouched, `.bak` created.
- Restart again with a now-complete file → no `.bak` churn, file unchanged.

### Unit Testing

- All sidecar keys absent → after call, file has all five with built-in defaults; `waybar:` section still present.
- Partial section (`log-level: debug` only) → other four added; `log-level` stays `debug`.
- All keys present → no change; `config.yaml.bak` is **not** created.
- User non-default value (e.g. `daemon: true`) → preserved after fill.
- Comment in sidecar section survives the rewrite.
- Backup created on change → `.bak` exists with original content.
- Missing file → no error, no file created.

### Integration Testing

- Launch with `HOME` pointing at an empty temp dir (after `ensureConfig` writes the embedded default): `ensureSettings` finds all five keys present → no `.bak` produced.
- Launch with a hand-crafted `config.yaml` that omits `notify-level`: after startup `notify-level: error` appears in the file.

## Status

Implemented.
