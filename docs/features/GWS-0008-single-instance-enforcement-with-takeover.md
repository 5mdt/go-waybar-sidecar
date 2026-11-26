# GWS-0008. Single-instance enforcement with takeover

**Tags:** #process-lifecycle #filesystem

## Description

On startup, the sidecar guarantees exactly one instance survives. The lock file holds the running instance's PID; a newcomer terminates that PID and takes over the lock. The newest instance always wins: the incumbent is terminated, not the newcomer.

Why: an exclusive lock cannot be raced, and the PID in it removes the need to scan `/proc` by executable name (which breaks for a renamed binary).

## Implementation

`pid.go`:
- `acquireInstanceLock() (f *os.File, holder int, err error)`: opens `<pidDir>/waybar-sidecar.lock` and takes a non-blocking exclusive `flock`.
  - Acquired: truncates the file and writes `os.Getpid()`; returns `f`.
  - `EWOULDBLOCK`: reads the PID in the file and returns `f=nil` and `holder` (0 if empty or unparsable). Not an error.
- `terminatePIDs(pids []int)`: sends `SIGTERM` to each pid, polls (`sidecarStopPoll`, 100ms) for up to `sidecarStopGrace` (5s) for all to exit, then sends `SIGKILL` to any still alive (logged at warn). Uses `killFn`. Shared with GWS-0022.
- `isRealProc(pid int, exeSuffix string) bool`: live (non-zombie, via `/proc/<pid>/status`) and `/proc/<pid>/exe` ends in `exeSuffix`. Used by GWS-0022.
- `ensureSingleInstance()` loops until `sidecarStopGrace` expires:
  1. `acquireInstanceLock()`; on success stores the file in `instanceLock` (held for the process lifetime; the kernel releases it on exit, even on a hard kill) and returns.
  2. If the holder is a PID not yet signalled, `terminatePIDs` it and retry at once. The PID is re-read every round, so a holder that had not yet written it is still caught.
  3. Otherwise sleep `sidecarStopPoll` and retry. After the deadline, log a warning and proceed unlocked rather than hang.

## Configuration

- `pidDir` = `~/.local/state/waybar`.
- `sidecarStopGrace` (5s) / `sidecarStopPoll` (100ms): internal timing vars, not user-configurable (mirrors `usr2SettleDelay` etc. in `waybar.go`).

## Testing

### Human Testing

- Start one instance, then start a second: the first logs a shutdown message and exits; the second logs the takeover and keeps running.
- Start three instances in quick succession (e.g. `for i in 1 2 3; do go-waybar-sidecar & done`), wait: exactly one `go-waybar-sidecar` process remains, and the lock file holds its PID.
- `kill -SIGSTOP` a running instance, then start a new one: the new one logs the SIGTERM timeout, then a SIGKILL, and the stopped process is gone (`kill -0 <pid>` fails).
- `--daemon` mode: after the fork, exactly one process remains, with no spurious "killed stray" log entries about itself.

### Unit Testing

- `isRealProc`: current process matches its own real exe suffix; does not match an unrelated suffix (e.g. `/waybar`); a nonexistent PID returns false without panicking.
- `acquireInstanceLock`: a first call succeeds and writes the caller's PID; a second, independent attempt on the same path (separate file descriptor) returns `f=nil` and the PID in the file, with no error.
- `terminatePIDs`: with a stubbed `killFn` that reports a pid as gone after the first liveness probe, only `SIGTERM` is sent. With one that always reports it alive, `SIGTERM` is sent, then - after the grace window - `SIGKILL`, and a warning is logged.
- `ensureSingleInstance`: with a free lock, no signals are sent and the lock is acquired. With the lock held by a descriptor whose file contains PID 4242 and a stubbed `killFn` that releases it on `SIGTERM`, 4242 receives `SIGTERM` and the lock is acquired with the caller's PID in the file. With the lock held and an empty file, it logs a warning and returns instead of hanging.

### Integration Testing

- Launch the binary three times in immediate succession against a temp `pidDir`/config: assert exactly one process remains alive after startup settles, and it holds the lock file.
- Launch a second instance while the first ignores `SIGTERM` (stopped via `SIGSTOP`): assert the second instance still starts, and the first is gone (via `SIGKILL`) rather than merely warned about.

## Status

Implemented.
