# GWS-0017. Fault-tolerant config reload

**Tags:** #config #reload #process-lifecycle

## Description

The sidecar is a long-running daemon and must not exit because of a bad `config.yaml` edit or a failed write to `outJSON`/menu XML. Load, parse, and output-write errors are logged and handled without crashing:

- At startup, if `config.yaml` exists but fails to load/parse, the sidecar falls back to the embedded default config (see GWS-0004) so it still starts and keeps watching for a fix.
- During live reload (GWS-0005), if the edited `config.yaml` fails to load/parse, the previous good config is kept and generation runs against it.
- If writing a menu XML file or `config.jsonc` fails, the error is logged and generation continues/returns without exiting.

## Implementation

`config.go`:
- `parseConfig(b []byte) (Config, error)` holds the YAML-unmarshal and `menu_definition` parsing; it returns errors, never exits.
- `loadConfig() (Config, error)` reads `configPath`, calls `parseConfig()` and returns any read/parse error to the caller.

`generator.go`:
- `buildXML()`: a `WriteToFile` failure for one menu is logged at error and that menu is skipped (`continue`); other menus are still written.
- `writeJSON()`: a failure to create or encode `outJSON` is logged at error and the function returns without exiting.

`watcher.go`:
- `reload(cfg Config) Config`. When `!noGenerate`, it calls `loadConfig()`; on error it logs "reload failed, keeping previous config" and returns `cfg` unchanged without regenerating. On success it generates from and returns the new config.

`main.go`:
- `run()` startup: if `loadConfig()` returns an error, it logs the error and falls back to `parseConfig(defaultConfig)` (the embedded default from GWS-0004) so `cfg` is always valid before the first `generate()`/`watch()`.

## Configuration

None (behavioral change only).

## Testing

### Human Testing

- With the sidecar running, edit `config.yaml` to contain invalid YAML and save: confirm an error is logged, the sidecar keeps running, and `config.jsonc`/menu XML are unchanged. Fix the file and confirm it reloads and regenerates normally.
- Start the sidecar with an existing but invalid `config.yaml`: confirm an error is logged, the embedded default config is used, and the sidecar starts and produces output from the default config.

### Unit Testing

- `parseConfig()` with malformed YAML: returns a non-nil error.
- `parseConfig(defaultConfig)`: succeeds (guards the startup fallback).
- `loadConfig()` with a missing/unreadable file: returns a non-nil error instead of exiting.
- `reload(cfg)` with `configPath` pointing at a broken file: the returned config equals `cfg` and `doReload`/`generate` are not invoked (no regeneration).

### Integration Testing

- None.

## Status

Implemented.
