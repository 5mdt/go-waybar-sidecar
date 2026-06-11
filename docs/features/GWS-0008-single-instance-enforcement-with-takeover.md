# GWS-0008. Single-instance enforcement with takeover

## Description

On startup, the sidecar checks for an already-running instance via its pidfile.
If one is found, the new process signals it to exit and takes over, instead of
exiting itself.

## Implementation

`pid.go` — `ensureSingleInstance()`:
- Reads `~/.local/state/waybar/waybar-sidecar.pid`.
- Verifies the PID belongs to a running `go-waybar-sidecar` process via
  `/proc/<pid>/exe`.
- Sends `SIGTERM` to the old process.
- Polls (`kill(pid, 0)`) every 100ms for up to 5s for it to exit.
- Proceeds with startup regardless of the outcome.

The old process handles `SIGTERM` in `main.go` (`run()`) and exits gracefully
(GWS-0012).

## Configuration

None.

## Testing

### Human Testing

- Start one instance, then start a second: the first should log a shutdown
  message and exit; the second should log "replacing running instance" and
  keep running.
- Verify `~/.local/state/waybar/waybar-sidecar.pid` ends up with the new PID.
- Start a second instance while the first is unresponsive (e.g. stopped with
  `SIGSTOP`): confirm the new instance logs a timeout warning after ~5s and
  still starts.

### Unit Testing

- `ensureSingleInstance()` with a pidfile pointing at a PID whose
  `/proc/<pid>/exe` does not end in `/go-waybar-sidecar`: should return
  immediately without signaling.
- `ensureSingleInstance()` with a pidfile containing the current process's own
  PID: should return immediately without signaling.
- `ensureSingleInstance()` with a missing or malformed pidfile: should return
  immediately.

### Integration Testing

- Launch the binary twice against a temp `pidDir`/config: assert the first
  process exits after receiving `SIGTERM` and the second remains running with
  its PID written to the pidfile.
- Launch a second instance while the first ignores `SIGTERM` (e.g. stopped via
  `SIGSTOP`): assert the second instance still starts after the ~5s timeout.

## Status

Implemented.
