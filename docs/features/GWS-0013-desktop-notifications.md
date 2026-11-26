# GWS-0013. Desktop notifications via D-Bus

**Tags:** #logging

## Description

The sidecar can mirror its log output to desktop notifications, so problems are visible even when running in `--daemon` mode (where stdio is closed and file logs aren't watched). Notifications are driven by the same log records as `log/slog` - a configurable level threshold decides which records are also sent to the user's notification daemon (mako, swaync, etc.) over the freedesktop notifications D-Bus interface. The feature is best-effort: if the session bus is unreachable or the daemon doesn't support the interface, text logging is unaffected and a single info line reports that notifications are inactive.

## Implementation

`src/notify.go`:
- `urgencyHint(level slog.Level) byte` maps `slog.LevelError` → `2` (critical), `slog.LevelWarn` → `1` (normal), and `info`/`debug` → `0` (low) - the `urgency` hint values used by `org.freedesktop.Notifications.Notify`.
- `parseNotifyLevel(s string) (level slog.Level, enabled bool, err error)` parses `--notify-level`. `"off"` (case-insensitive) returns `enabled=false`; any other valid level (`debug`, `info`, `warn`, `error`) delegates to `slog.Level.UnmarshalText` and returns `enabled=true`. An unrecognized value returns an error.
- `notifier` owns a `github.com/godbus/dbus/v5` session-bus connection and the set of notification IDs currently awaiting a click:
  - `newNotifier(appName string) (*notifier, bool)` connects to the session bus and checks `GetCapabilities` for `"actions"`; returns `ok=false` if either step fails, meaning notifications stay text-only.
  - `send()` calls `Notify` with a single `["default", label]` action and records `id -> path` so a later click can be resolved.
  - a background goroutine (started by `newNotifier`) receives `ActionInvoked` and `NotificationClosed` signals: on `ActionInvoked` it looks up the path and calls `openTarget`; on `NotificationClosed` it drops the ID so the pending map can't grow unbounded.
- `notification{urgency, summary, body, label, path}` is the queued unit; see GWS-0026 for the click-action attribute behind `label`/`path`.
- `openTarget` (var, stubbable) runs `xdg-open <path>` to fulfil a clicked action.
- `notifyHandler` is a `slog.Handler` that, for records at or above its threshold, does a non-blocking send of a `notification` (record message as summary, formatted attributes as body) to its queue. When the queue is full the record is dropped, and `Handle` always returns `nil`, so a hung or absent notification daemon never blocks or breaks logging.

`src/main.go`:
- Flag `--notify-level <level>` (default `"error"`).
- `buildLogger()` always builds the `TextHandler`. If notifications are enabled and `newNotifier` succeeds, it also builds a `notifyHandler` and combines both via `slog.NewMultiHandler`. The queue is a `chan notification` drained by one goroutine calling `notifier.send`; send errors are ignored.
- If `newNotifier` fails, an info line ("desktop notifications inactive") is logged once and text logging continues.

## Configuration

- `--notify-level`: `off` | `debug` | `info` | `warn` | `error` (default `error`). Selects the minimum log level that is also sent as a desktop notification. `off` disables the feature entirely (no bus connection is opened).

## Testing

### Human Testing

- Run with default flags, force a waybar cold-start failure (e.g. rename the `waybar` binary temporarily): confirm a **critical** desktop notification appears.
- Run with `--notify-level info`, edit `config.yaml`: confirm a **low** urgency notification appears for "output files written" and a **low/normal** notification for the reload outcome.
- Run with `--notify-level off`: confirm no notifications appear while text logs continue normally.
- Run with `DBUS_SESSION_BUS_ADDRESS` unset/invalid: confirm the process starts normally, text logging works, and a single info line states notifications are inactive.
- Run with `--daemon --log-output <file>` and trigger an error: confirm the notification still appears.

### Unit Testing

- `urgencyHint`: error → `2`, warn → `1`, info/debug → `0`.
- `parseNotifyLevel`: `"off"` → disabled, no error; `"error"`/`"warn"`/ `"info"`/`"debug"` → enabled with the matching level; invalid string → error.
- `notifyHandler.Enabled`: true only when the record level is >= threshold.
- `notifyHandler.Handle`: a record at/above threshold queues a notification with the expected urgency, summary, and body; a record below threshold queues nothing.
- `notifyHandler.Handle` resilience: with a full queue (or none), `Handle` returns `nil` immediately without blocking.
- notifier action dispatch (see GWS-0026 for the click-action attribute itself): a known notification ID resolves to the recorded path and calls `openTarget`; an unknown ID does nothing; `NotificationClosed` removes the ID from the pending set.

### Integration Testing

- Build the binary and run it with `--notify-level off`: assert no D-Bus connection is attempted while log output is unaffected.
- Run with `--notify-level error` against a real or test session bus: trigger an error-level event and assert `Notify` was called with urgency `2`.

## Status

Implemented.
