# GWS-0006. Waybar reload via SIGUSR2

**Tags:** #reload #process-lifecycle

## Description

After regenerating `config.jsonc` and the per-menu XML files (GWS-0015), the
sidecar asks a running
Waybar instance to reload its configuration in place by sending `SIGUSR2`,
avoiding a full restart when possible.

## Implementation

`waybar.go` `reloadWaybar()`, called at the end of `generate()`:
- `waybarPID()` finds the running Waybar process via `pgrep -x waybar`,
  taking the first PID from the output and validating it with
  `isRealWaybar()` (not a zombie, and `/proc/<pid>/exe` ends in `/waybar`).
- If a valid PID is found, sends `SIGUSR2` to it. On success:
  - Logs "waybar reloaded via USR2" and saves the PID via `savePID("waybar",
    pid)` (GWS-0009).
  - Waits 500ms, then re-checks `waybarPID()`. If Waybar is still running,
    returns - reload succeeded.
  - If Waybar is no longer running, logs a warning and falls through to cold
    start (GWS-0007).
- If no valid PID is found, or `SIGUSR2` fails, falls through to cold start.

## Configuration

None.

## Testing

### Human Testing

- With Waybar running, change `config.yaml`, let the sidecar regenerate, and
  confirm Waybar's bars update without the process restarting (same PID
  before/after, visible via `pgrep -x waybar`).
- Confirm a log line "waybar reloaded via USR2" with the correct PID appears.

### Unit Testing

- `isRealWaybar()` with a PID whose `/proc/<pid>/exe` does not end in
  `/waybar`: assert `false`.
- `isRealWaybar()` with a PID in zombie state (`State: Z` in
  `/proc/<pid>/status`): assert `false`.
- `waybarPID()` when `pgrep -x waybar` returns no output: assert `0`.

### Integration Testing

- Launch a long-running stub binary named `waybar` that ignores `SIGUSR2`,
  run `reloadWaybar()`, and assert it logs a successful reload and
  `savePID("waybar", ...)` is called with the stub's PID.
- Launch a stub `waybar` that exits on `SIGUSR2`, run `reloadWaybar()`, and
  assert it falls through to the cold-start path (GWS-0007).

## Status

Implemented.
