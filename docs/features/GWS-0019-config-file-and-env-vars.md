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
- `sidecarSettings` — pointer-field struct (`*string`/`*bool`) so a missing
  YAML key is `nil` (absent) rather than a zero value.
- `rawConfig.Sidecar *sidecarSettings` — unmarshals the `sidecar:` top-level
  key alongside the existing `waybar:` key.
- `loadSettings(path string) sidecarSettings` — reads the config file; returns
  an empty struct (no error) when the file is missing (first run) or has no
  `sidecar:` key.
- `resolveConfigPath(f cliFlags, set map[string]bool) string` — applies the
  two-source precedence for the config path: default → env
  (`GO_WAYBAR_SIDECAR_CONFIG`) → CLI flag (`--config`).
- `resolveSettings(f cliFlags, set map[string]bool, s sidecarSettings) resolvedSettings`
  — applies four-source precedence for the other five settings. Env vars for
  booleans accept `true/false/yes/no/1/0` (case-insensitive); `yes`/`no` extend `strconv.ParseBool`; invalid
  values are silently ignored (config-file or default takes effect).

`main.go`:
- `parseFlags()` returns `(cliFlags, map[string]bool)`. The set is built via
  `flag.Visit` after `flag.Parse()` so only explicitly-passed flags win over
  env/file.
- `main()` applies the resolution order:
  1. `resolveConfigPath` to set `configPath`.
  2. `loadSettings(configPath)` (tolerates missing file).
  3. `resolveSettings` for the remaining five values.
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

- `loadSettings`: valid `sidecar:` section parses correctly; missing file →
  empty struct; file without `sidecar:` key → all-nil fields.
- `resolveSettings` (table-driven): default-only, file-overrides-default,
  env-overrides-file, flag-overrides-env for each setting.
- `resolveConfigPath`: default → env → flag precedence.
- Bool env parsing: `true/false/yes/no/1/0` accepted; invalid value ignored.

### Integration Testing

- Launch with `HOME` pointing to an empty temp dir and no flags/env: app starts
  on defaults, `config.yaml` is created with a `sidecar:` block.
- Launch with `sidecar.log-level: debug` in config.yaml and
  `GO_WAYBAR_SIDECAR_LOG_LEVEL=warn`: `warn` level is active.

## Status

Implemented.
