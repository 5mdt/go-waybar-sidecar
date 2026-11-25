# GWS-0019. Config-file and environment-variable settings

**Tags:** #config #cli

## Description

Runtime settings (`log-level`, `log-output`, `notify-level`, `no-generate`,
`daemon`) can now be set persistently via the `sidecar:` section of
`config.yaml`, and overridden per-session via environment variables, following
the precedence:

```
defaults  <  config file (sidecar:)  <  environment variables  <  CLI flags
```

`--config` (the path to the config file itself) is excluded from the config
file to avoid a chicken-and-egg problem, but it does accept an env var.

## Implementation

`config.go`:
- `settings` — one table row per setting: key, env var, default (string), flag
  usage, `isBool`. Flags, env lookup, file lookup, `--help` text, and the
  GWS-0020 defaults are all driven by it, so adding a setting is one row.
- `rawConfig.Sidecar map[string]string` — the `sidecar:` top-level key next
  to `waybar:`; scalars (including YAML `true`) decode to their text.
- `loadSettings(path string) (map[string]string, error)` — reads the
  `sidecar:` section. A missing file (first run) or absent key gives `nil, nil`;
  unreadable or malformed YAML returns the error.
- `resolveConfigPath(fs *flag.FlagSet) string` — default → env
  (`GO_WAYBAR_SIDECAR_CONFIG`) → `--config` if passed.
- `resolveSettings(fs *flag.FlagSet, file map[string]string) (resolvedSettings, error)`
  — for each table row takes the last of default, file, env, flag (flag only
  if passed, found via `fs.Visit`). Booleans accept `true/false/yes/no/1/0`
  (case-insensitive; `yes`/`no` extend `strconv.ParseBool`); an invalid
  boolean from any source is an error.

`main.go`:
- `newFlagSet()` registers one flag per table row plus `--config`.
  `--help` prints the flag defaults from the flag set, then env vars and the
  `sidecar:` block generated from the table.
- `main()` applies the resolution order:
  1. `resolveConfigPath` to set `configPath`.
  2. `loadSettings(configPath)`; on error it logs a warning and continues on
     defaults.
  3. `resolveSettings`; on error it logs and exits 1 (as for an invalid
     level).
- `forkDetach()` sets `GO_WAYBAR_SIDECAR_FORKED=1` in the child's environment.
  The daemon check becomes `if resolved.daemon && os.Getenv("GO_WAYBAR_SIDECAR_FORKED") == ""`
  so that a `daemon: true` in the config file or `GO_WAYBAR_SIDECAR_DAEMON=1`
  does not cause the child to fork again (infinite loop).

## Configuration

### `sidecar:` section in `config.yaml`

```yaml
sidecar:
  log-level: info          # debug | info | warn | error
  log-output: stdout       # stdout | stderr | <file path>
  notify-level: error      # off | debug | info | warn | error
  no-generate: false
  daemon: false
```

All keys are optional; omitted keys fall back to the defaults above.

### Environment variables

| Variable | Overrides | Values |
|----------|-----------|--------|
| `GO_WAYBAR_SIDECAR_LOG_LEVEL` | `--log-level` | `debug`, `info`, `warn`, `error` |
| `GO_WAYBAR_SIDECAR_LOG_OUTPUT` | `--log-output` | `stdout`, `stderr`, file path |
| `GO_WAYBAR_SIDECAR_NOTIFY_LEVEL` | `--notify-level` | `off`, `debug`, `info`, `warn`, `error` |
| `GO_WAYBAR_SIDECAR_NO_GENERATE` | `--no-generate` | `true`/`false`/`1`/`0` |
| `GO_WAYBAR_SIDECAR_DAEMON` | `--daemon` | `true`/`false`/`1`/`0` |
| `GO_WAYBAR_SIDECAR_CONFIG` | `--config` | file path |

## Testing

### Human Testing

- No flags, no env, no `sidecar:` in config.yaml → app runs on defaults (info
  log, stdout, error notify, no daemon, generate).
- Add `sidecar:\n  log-level: debug` to config.yaml, restart → debug logs.
- `GO_WAYBAR_SIDECAR_LOG_LEVEL=warn` with `log-level: debug` in file → warn wins.
- `--log-level=error GO_WAYBAR_SIDECAR_LOG_LEVEL=warn` → error wins.
- `GO_WAYBAR_SIDECAR_DAEMON=1` → forks exactly once; child process does not
  fork again.
- `GO_WAYBAR_SIDECAR_CONFIG=/tmp/alt.yaml` → that path is used; `--config`
  overrides it.

### Unit Testing

- `loadSettings`: valid `sidecar:` section (including YAML `true` and `yes`
  booleans) parses correctly; missing file or absent key → nil map, no error;
  malformed YAML → error.
- `resolveSettings`: default-only, file-overrides-default, env-overrides-file,
  flag-overrides-env for each setting.
- `resolveConfigPath`: default → env → flag precedence.
- Bool parsing: `true/false/yes/no/1/0` accepted from env; an invalid
  boolean from env, file, or flag is an error.

### Integration Testing

- Launch with `HOME` pointing to an empty temp dir and no flags/env: app starts
  on defaults, `config.yaml` is created with a `sidecar:` block.
- Launch with `sidecar.log-level: debug` in config.yaml and
  `GO_WAYBAR_SIDECAR_LOG_LEVEL=warn`: `warn` level is active.

## Status

Implemented.
