# GWS-0013. Desktop notifications via D-Bus

**Tags:** #logging

## Description

The sidecar can mirror its log output to desktop notifications, so problems
are visible even when running in `--daemon` mode (where stdio is closed and
file logs aren't watched). Notifications are driven by the same log records as
`log/slog` - a configurable level threshold decides which records are also
sent to the user's notification daemon (mako, swaync, etc.) over the
freedesktop notifications D-Bus interface. The feature is best-effort: if the
session bus is unreachable or the daemon doesn't support the interface, text
logging is unaffected and a single info line reports that notifications are
inactive.

## Implementation

`notify.go`:
- `urgencyHint(level slog.Level) byte` maps `slog.LevelError` → `2`
  (critical), `slog.LevelWarn` → `1` (normal), and `info`/`debug` → `0` (low) -
  the `urgency` hint values used by `org.freedesktop.Notifications.Notify`.
- `parseNotifyLevel(s string) (level slog.Level, enabled bool, err error)`
  parses `--notify-level`. `"off"` (case-insensitive) returns `enabled=false`;
  any other valid level (`debug`, `info`, `warn`, `error`) delegates to
  `slog.Level.UnmarshalText` and returns `enabled=true`. An unrecognized value
  returns an error.
- `notifier` owns a `github.com/godbus/dbus/v5` session-bus connection and the
  set of notification IDs currently awaiting a click:
  - `newNotifier(appName string) (*notifier, bool)` connects to the session
    bus and checks `GetCapabilities` for `"actions"`; returns `ok=false` if
    either step fails, meaning notifications stay text-only.
  - `send(urgency byte, summary, body, label, path string) error` calls
    `Notify` with a single `["default", label]` action and records
    `id -> path` so a later click can be resolved.
  - a background goroutine (started by `newNotifier`) receives
    `ActionInvoked` and `NotificationClosed` signals: on `ActionInvoked` it
    looks up the path and calls `openTarget`; on `NotificationClosed` it drops
    the ID so the pending map can't grow unbounded.
- `sendNotification` (var, stubbable) is bound to a `notifier`'s `send` method;
  see GWS-0026 for the click-action attribute this depends on.
- `openTarget` (var, stubbable) runs `xdg-open <path>` to fulfil a clicked
  action.
- `notifyHandler` is a `slog.Handler` that, for records at or above its
  threshold, calls `sendNotification` with the record message as the
  notification summary and the formatted attributes as the body. `Handle`
  always returns `nil`: any `sendNotification` error is swallowed so a broken
  or absent notification daemon never affects logging or causes a fatal error.
- `multiHandler` is a `slog.Handler` that fans `Handle`/`WithAttrs`/
  `WithGroup` out to multiple child handlers, gating each child on its own
  `Enabled`.

`main.go`:
- New flag `--notify-level <level>` (default `"error"`): `off`, `debug`,
  `info`, `warn`, or `error`.
- `buildLogger(level, output, notifyLevel, notifyEnabled)` always builds the
  existing `TextHandler`. If `notifyEnabled`, it calls `newNotifier` and, if
  that succeeds, also builds a `notifyHandler` and combines both via
  `multiHandler`. Otherwise the text handler is used unchanged.
- If notifications are requested (`notifyEnabled == true`) but `newNotifier`
  fails, an info line ("desktop notifications inactive") is logged once at
  startup and the process continues normally with text logging only.
- `--daemon` mode re-execs with the inherited environment, so
  `DBUS_SESSION_BUS_ADDRESS` / `XDG_RUNTIME_DIR` are available to the
  notifier in the forked process.

## Configuration

- `--notify-level`: `off` | `debug` | `info` | `warn` | `error` (default
  `error`). Selects the minimum log level that is also sent as a desktop
  notification. `off` disables the feature entirely (no bus connection is
  opened).

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
- Run with `DBUS_SESSION_BUS_ADDRESS` unset/invalid: confirm the process
  starts normally, text logging works, and a single info line states
  notifications are inactive.
- Run with `--daemon --log-output <file>` and trigger an error: confirm the
  notification still appears.

### Unit Testing

- `urgencyHint`: error → `2`, warn → `1`, info/debug → `0`.
- `parseNotifyLevel`: `"off"` → disabled, no error; `"error"`/`"warn"`/
  `"info"`/`"debug"` → enabled with the matching level; invalid string →
  error.
- `notifyHandler.Enabled`: true only when the record level is >= threshold.
- `notifyHandler.Handle`: a record at/above threshold calls
  `sendNotification` with the expected app name, urgency, summary, and body;
  a record below threshold does not call it.
- `notifyHandler.Handle` resilience: when the stubbed `sendNotification`
  returns an error, `Handle` still returns `nil`.
- notifier action dispatch (see GWS-0026 for the click-action attribute
  itself): a known notification ID resolves to the recorded path and calls
  `openTarget`; an unknown ID does nothing; `NotificationClosed` removes the
  ID from the pending set.

### Integration Testing

- Build the binary and run it with `--notify-level off`: assert no D-Bus
  connection is attempted while log output is unaffected.
- Run with `--notify-level error` against a real or test session bus: trigger
  an error-level event and assert `Notify` was called with urgency `2`.

## Status

Implemented.
