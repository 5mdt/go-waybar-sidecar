# GWS-0008. Single-instance enforcement with takeover

**Tags:** #process-lifecycle #filesystem

## Description

On startup the newest instance always wins: it terminates the running one and takes over its lock file.

## Implementation

`pid.go` - `ensureSingleInstance()`:
- `acquireInstanceLock()` takes a non-blocking exclusive `flock` on `<pidDir>/waybar-sidecar.lock` and writes its PID there.
- If the lock is held, `terminatePIDs()` sends `SIGTERM` to the PID in the file, then `SIGKILL` after `sidecarStopGrace` (5s), and retries.
- After the deadline it logs a warning and proceeds unlocked.

## Configuration

None.

## Testing

### Human Testing

- Start a second instance: the first logs a shutdown message and exits, the second keeps running.

### Unit Testing

- `acquireInstanceLock`: a second attempt on the same path returns the holder PID and no error.
- `terminatePIDs` with a stubbed `killFn` that never reports the PID gone: `SIGTERM`, then `SIGKILL`.

### Integration Testing

None.

## Status

Implemented.
