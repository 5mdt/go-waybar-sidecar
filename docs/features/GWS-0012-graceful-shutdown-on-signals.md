# GWS-0012. Graceful shutdown on signals

## Description

The sidecar handles `SIGINT`, `SIGTERM`, and `SIGHUP` so it can be stopped
cleanly (or replaced, per GWS-0008) without leaving partial state.

## Implementation

`main.go` `run()`:
- Registers a buffered signal channel via `signal.Notify(sigs, syscall.SIGINT,
  syscall.SIGTERM, syscall.SIGHUP)` and handles signals in a goroutine.
- `SIGHUP`: logged at debug ("SIGHUP received (ignored)") and otherwise
  ignored — the sidecar does not use `SIGHUP` for reload (config reload is
  file-watch based, see GWS-0005).
- `SIGINT` / `SIGTERM`: logs "shutting down" with the signal name and calls
  `os.Exit(0)`.
- This is the same signal path used when `ensureSingleInstance()` (GWS-0008)
  sends `SIGTERM` to a previous instance during takeover.
- No explicit cleanup is performed on shutdown (no deferred unlocks/temp file
  removal); the process simply exits.

## Configuration

None.

## Testing

### Human Testing

- Send `SIGINT` (Ctrl-C) to a running sidecar: confirm it logs "shutting
  down" with `signal=interrupt` and exits with code 0.
- Send `SIGTERM`: confirm the same with `signal=terminated`.
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
