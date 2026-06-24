# GWS-0022. Reap duplicate waybar instances

**Tags:** #process-lifecycle #reload

## Description

When more than one valid waybar process is running simultaneously, the sidecar
keeps the newest one (by kernel start time) and sends `SIGTERM` to each older
instance. This prevents stale/duplicate bars accumulating after session restores,
crashes, or race conditions during reload.

Terminating the sidecar itself never affects waybar: waybar is detached in its
own session (GWS-0007) and sidecar shutdown performs no waybar signaling (GWS-0012).

## Implementation

`waybar.go` `reapDuplicateWaybars()`:
- `waybarPIDs()` enumerates all PIDs from `pgrep -x waybar`, filtered by the
  existing `isRealWaybar()` (non-zombie, `/proc/<pid>/exe` ends in `/waybar`).
- If `<= 1` valid PID found, returns immediately (no-op).
- Determines the newest instance by reading field 22 (`starttime`) from
  `/proc/<pid>/stat`, after the last `)` (guards against process names that
  contain spaces or parentheses).
- Sends `SIGTERM` to each non-newest PID (positive pid only — never a process
  group). Graceful only; no SIGKILL escalation. Proceeds regardless of whether
  the signal succeeds (mirrors `ensureSingleInstance` in `pid.go`). Logs
  "killed older waybar instance" with `pid` and `kept`.
- If start time cannot be read for a PID, that PID is skipped (treated as
  unknown age).

Called:
- At the top of `reloadWaybar()` (before SIGUSR2 or cold-start logic).
- Once in `main.go` `run()` at startup, after `ensureSingleInstance()` /
  `savePID()`, so pre-existing duplicates are cleaned up without waiting for a
  config change.

## Configuration

None.

## Testing

### Human Testing

- Start two waybars (`waybar & sleep 1; waybar &`), trigger a config change or
  restart the sidecar. Confirm the log contains "killed older waybar instance"
  and `pgrep -x waybar` shows exactly one PID (the newer one).
- With one waybar running, confirm no "killed older waybar instance" log on
  normal reload.
- Kill the sidecar with `SIGTERM` while waybar is running: confirm
  `pgrep -x waybar` still shows the waybar PID alive.

### Unit Testing

- Two PIDs with known start times: assert the older receives `SIGTERM`, the
  newer does not.
- Three PIDs with start times in non-monotonic order: assert the one with the
  highest start time is kept, the other two receive `SIGTERM`.
- Single PID: assert zero kills.
- No PIDs: assert zero kills.
- `procStartTime(os.Getpid())` returns a non-zero value with no error.

### Integration Testing

- With a stub `waybar` binary on `PATH`, launch two instances with a
  brief delay, run `reapDuplicateWaybars()`, and assert only the newer
  instance remains running.

## Status

Implemented.
