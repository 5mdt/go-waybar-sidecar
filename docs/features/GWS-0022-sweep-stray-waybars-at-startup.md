# GWS-0022. Sweep stray waybar instances at startup

**Tags:** #process-lifecycle #reload

## Description

At startup, before the sidecar has spawned anything, any `waybar` process
found on the system is a stray from a previous session (a crash, or the
previous instance's takeover, see GWS-0008) - the sidecar owns none of them
yet, so all are terminated before it starts its own.

Once running, the sidecar's own waybar is a supervised child (GWS-0027):
`reloadWaybar()` deals with it directly (SIGUSR2 or stop+start) and never
needs to re-discover or arbitrate between multiple processes, since there is
only ever the one it started.

## Implementation

`waybar.go` `sweepStrayWaybars()`:
- `waybarPIDs()` enumerates all PIDs from `pgrep -x waybar`, filtered by
  `isRealWaybar()` (non-zombie, `/proc/<pid>/exe` ends in `/waybar`).
- Sends `SIGTERM` to every one found. Graceful only; no `SIGKILL` escalation
  (mirrors `ensureSingleInstance` in `pid.go`) - these are pre-existing
  processes, not ones the sidecar is actively supervising, so there is no
  `stop()` grace/escalation logic to reuse. Logs "killed stray waybar
  instance" per PID.

Called once, from `main.go` `run()` at startup, after `ensureSingleInstance()`
/ `savePID()` and before the first `generate()`/`reloadWaybar()` - so
pre-existing strays are cleared before the sidecar starts its own instance.

## Configuration

None.

## Testing

### Human Testing

- Start a stray waybar manually (`waybar &`), then start the sidecar. Confirm
  the log contains "killed stray waybar instance" and, once the sidecar has
  finished starting its own, `pgrep -x waybar` shows exactly one PID - the
  sidecar's.
- With no waybar running, start the sidecar: confirm no "killed stray waybar
  instance" log.

### Unit Testing

- Stubbed `waybarPIDsFn` returning several PIDs: every one receives
  `SIGTERM`.
- Stubbed `waybarPIDsFn` returning none: zero kills, no log line.

### Integration Testing

- With a stub `waybar` binary on `PATH`, launch an instance, then start the
  sidecar and run `sweepStrayWaybars()`: assert the stray is terminated before
  the sidecar's own instance comes up.

## Status

Implemented.
