# GWS-0026. Notification click actions

**Tags:** #logging

## Description

Desktop notifications (GWS-0013) are clickable: every notification carries a single action that opens a relevant file or directory with `xdg-open`. Clicking the notification body (or its action button) opens that target so the user can jump straight to the config, menu file, or job output that triggered the log record, instead of hunting for it by hand. A log call site can name its own target; otherwise the notification opens `config.yaml`.

## Implementation

`notify.go`:
- `notifyOpen(label, path string) slog.Attr` returns a `slog.Attr` keyed `notify_open` wrapping an `openAction{label, path}`. Call sites pass it like any other log attribute, e.g. `logger.Warn("job failed", "name", j.Name, "err", err, notifyOpen("Show output", outPath))`.
- `openAction` implements `slog.LogValuer`, rendering as just its `path` in the text log - the label is notification-only, so text logs stay unaffected by this feature.
- `notifyHandler.Handle` scans the merged attrs (`h.attrs` + the record's own) for `notify_open`. If found, its label/path drive the notification's action and the attribute is excluded from the notification body (it already appears in the text log via `LogValue`). If absent, the default action is `notifyOpen("Open config", configPath)`.
- The resolved label/path are queued with the notification and threaded by `notifier.send` (GWS-0013) into the `Notify` action.

Call sites that override the default:
- `cron.go`, job failure: `notifyOpen("Show output", expandHome(j.Output))` when the job has an `output:` path.
- `generator.go`, invalid menu XML: `notifyOpen("Open menu", path)` for the offending menu file.

Every other notification (e.g. reload failures, waybar start failures) uses the default "Open config" action, since `config.yaml` is the first place to look.

## Configuration

None. Actions are derived from the log call site, not user-configurable.

## Testing

### Human Testing

- Trigger "job failed" for a job with `output:` set: confirm the notification button reads "Show output" and clicking it (or the notification body) opens that file.
- Break a menu's XML: confirm the notification offers "Open menu" for the offending file.
- Trigger any notification with no explicit action (e.g. a waybar start failure): confirm it offers "Open config" and opens `config.yaml`.

### Unit Testing

- `notifyHandler.Handle` with no `notify_open` attr: the queued notification has label `"Open config"` and `configPath`.
- `notifyHandler.Handle` with `notifyOpen("Show output", "/tmp/x")` in the record attrs: the queued notification has that label/path, and the notification body does not contain `notify_open=`.
- `openAction.LogValue`: returns the path only.

### Integration Testing

- Covered by GWS-0013's notifier action-dispatch tests (they exercise the path chosen here end to end).

## Status

Implemented.
