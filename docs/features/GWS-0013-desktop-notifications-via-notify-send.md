# GWS-0013. Desktop notifications via notify-send

## Description

The sidecar can mirror its log output to desktop notifications via
`notify-send`, so problems are visible even when running in `--daemon` mode
(where stdio is closed and file logs aren't watched). Notifications are driven
by the same log records as `log/slog` — a configurable level threshold decides
which records are also sent to the user's notification daemon (mako, swaync,
etc.). The feature is best-effort: if `notify-send` is unavailable, text
logging is unaffected and a single info line reports that notifications are
inactive.

## Implementation

`notify.go`:
- `urgencyForLevel(level slog.Level) string` maps `slog.LevelError` →
  `"critical"`, `slog.LevelWarn` → `"normal"`, and `info`/`debug` → `"low"`.
- `parseNotifyLevel(s string) (level slog.Level, enabled bool, err error)`
  parses `--notify-level`. `"off"` (case-insensitive) returns `enabled=false`;
  any other valid level (`debug`, `info`, `warn`, `error`) delegates to
  `slog.Level.UnmarshalText` and returns `enabled=true`. An unrecognized value
  returns an error.
- `lookNotifySend` (var, stubbable) wraps `exec.LookPath("notify-send")`;
  `notifyAvailable()` reports whether the binary is on `PATH`.
- `sendNotification` (var, stubbable) shells out to
  `notify-send -a <app> -u <urgency> <summary> [<body>]`.
- `notifyHandler` is a `slog.Handler` that, for records at or above its
  threshold, calls `sendNotification` with the record message as the
  notification summary and the formatted attributes as the body. `Handle`
  always returns `nil`: any `sendNotification` error is swallowed so a broken
  or absent `notify-send` never affects logging or causes a fatal error.
- `multiHandler` is a `slog.Handler` that fans `Handle`/`WithAttrs`/
  `WithGroup` out to multiple child handlers, gating each child on its own
  `Enabled`.

`main.go`:
- New flag `--notify-level <level>` (default `"error"`): `off`, `debug`,
  `info`, `warn`, or `error`.
- `buildLogger(level, output, notifyLevel, notifyEnabled)` always builds the
  existing `TextHandler`. If `notifyEnabled && notifyAvailable()`, it also
  builds a `notifyHandler` and combines both via `multiHandler`. Otherwise the
  text handler is used unchanged.
- If notifications are requested (`notifyEnabled == true`) but `notify-send`
  is not on `PATH`, an info line ("desktop notifications inactive") is logged
  once at startup and the process continues normally with text logging only.
- `--daemon` mode re-execs with the inherited environment, so
  `DBUS_SESSION_BUS_ADDRESS` / `XDG_RUNTIME_DIR` are available to
  `notify-send` in the forked process.

## Configuration

- `--notify-level`: `off` | `debug` | `info` | `warn` | `error` (default
  `error`). Selects the minimum log level that is also sent as a desktop
  notification. `off` disables the feature entirely.

## Testing

### Human Testing

- Run with default flags, force a waybar cold-start failure (e.g. rename the
  `waybar` binary temporarily): confirm a **critical** desktop notification
  appears.
- Run with `--notify-level info`, edit `~/.config/waybar/config.yaml`: confirm
  a **low** urgency notification appears for "output files written" and a
  **low/normal** notification for the reload outcome.
- Run with `--notify-level off`: confirm no notifications appear while text
  logs continue normally.
- Run with `notify-send` removed from `PATH`: confirm the process starts
  normally, text logging works, and a single info line states notifications
  are inactive.
- Run with `--daemon --log-output <file>` and trigger an error: confirm the
  notification still appears.

### Unit Testing

- `urgencyForLevel`: error → `critical`, warn → `normal`, info/debug →
  `low`.
- `parseNotifyLevel`: `"off"` → disabled, no error; `"error"`/`"warn"`/
  `"info"`/`"debug"` → enabled with the matching level; invalid string →
  error.
- `notifyHandler.Enabled`: true only when the record level is >= threshold.
- `notifyHandler.Handle`: a record at/above threshold calls
  `sendNotification` with the expected app name, urgency, summary, and body;
  a record below threshold does not call it.
- `notifyHandler.Handle` resilience: when the stubbed `sendNotification`
  returns an error, `Handle` still returns `nil`.
- `notifyAvailable()`: returns false when the stubbed lookup reports
  `notify-send` is missing, true otherwise.

### Integration Testing

- Build the binary and run it with `--notify-level off`: assert no calls to
  `notify-send` occur (e.g. by stubbing `PATH` to a directory containing a
  recording shim instead of the real binary) while log output is unaffected.
- Run with `--notify-level error` and a recording `notify-send` shim on
  `PATH`: trigger an error-level event and assert the shim was invoked with
  `-u critical`.

## Status

Implemented.
