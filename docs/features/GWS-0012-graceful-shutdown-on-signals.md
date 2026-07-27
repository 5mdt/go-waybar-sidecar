# GWS-0012. Graceful shutdown on signals

**Tags:** #process-lifecycle

## Description

The sidecar handles `SIGINT`, `SIGTERM`, and `SIGHUP` so it can be stopped
cleanly (or replaced, per GWS-0008) without leaving partial state.

## Implementation

`main.go` `run()`:
- Registers a buffered signal channel via `signal.Notify(sigs, syscall.SIGINT,
  syscall.SIGTERM, syscall.SIGHUP)` and handles signals in a goroutine.
- `SIGHUP`: logged at debug ("SIGHUP received (ignored)") and otherwise
  ignored - the sidecar does not use `SIGHUP` for reload (config reload is
  file-watch based, see GWS-0005).
- `SIGINT` / `SIGTERM`: logs "shutting down" with the signal name, calls
  `waybar.stop()` (GWS-0027) to stop the supervised waybar child, then calls
  `os.Exit(0)`.
- This is the same signal path used when `ensureSingleInstance()` (GWS-0008)
  sends `SIGTERM` to a previous instance during takeover - so the outgoing
  instance's waybar is stopped as part of its own shutdown, rather than being
  orphaned for the incoming instance to fight over.
- No other cleanup is performed on shutdown (no deferred unlocks/temp file
  removal); the process exits once waybar has been stopped.

## Configuration

None.

## Testing

### Human Testing

- Send `SIGINT` (Ctrl-C) to a running sidecar: confirm it logs "shutting
  down" with `signal=interrupt` and exits with code 0.
- Send `SIGTERM`: confirm the same with `signal=terminated`, and confirm the
  waybar process the sidecar started exits as well (no orphan left behind).
- Send `SIGHUP`: confirm it logs "SIGHUP received (ignored)" at debug level
  and the process keeps running.

### Unit Testing

- Not directly unit-testable (relies on process-level signal delivery and
  `os.Exit`); covered by integration tests below.

### Integration Testing

- Launch the binary, send `SIGTERM`, and assert the process exits with code 0
  and the log contains "shutting down".
- Launch the binary, send `SIGHUP`, and assert the process is still running
  afterward and the log contains "SIGHUP received (ignored)" (with
  `--log-level debug`).

## Status

Implemented.
