# GWS-0006. Waybar reload via SIGUSR2

**Tags:** #reload #process-lifecycle

## Description

After regenerating `config.jsonc` and the per-menu XML files (GWS-0015), the
sidecar asks a running
Waybar instance to reload its configuration in place by sending `SIGUSR2`,
avoiding a full restart when possible.

## Implementation

`waybar.go` `reloadWaybar()`, called at the end of `generate()`:
- `waybar.alive()` reports whether the sidecar's supervised child (GWS-0027)
  is currently running - an in-memory channel check, not a process lookup.
- If alive, sends `SIGUSR2` to `waybar.pid()`. On success:
  - Logs "waybar reloaded via USR2".
  - Waits `usr2SettleDelay` (500ms), then re-checks `waybar.alive()`. If still
    alive, returns - reload succeeded.
  - If no longer alive, logs a warning and falls through to cold start
    (GWS-0007).
- If not alive, falls through to cold start. If `SIGUSR2` fails, logs a
  warning and falls through.

## Configuration

None.

## Testing

### Human Testing

- With Waybar running, change `config.yaml`, let the sidecar regenerate, and
  confirm Waybar's bars update without the process restarting (same PID
  before/after, visible via `pgrep -x waybar`).
- Confirm a log line "waybar reloaded via USR2" with the correct PID appears.

### Unit Testing

See GWS-0027 for `waybarProc` unit tests (`alive`, `pid`, `stop`, `start`).
`reloadWaybar()` with a live child takes the SIGUSR2 path and spawns no new
process.

### Integration Testing

- Launch a long-running stub binary named `waybar` that ignores `SIGUSR2`,
  run `reloadWaybar()`, and assert it logs "waybar reloaded via USR2" with
  the stub's PID.
- Launch a stub `waybar` that exits on `SIGUSR2`, run `reloadWaybar()`, and
  assert it falls through to the cold-start path (GWS-0007).

## Status

Implemented.
