# GWS-0008. Single-instance enforcement with takeover

**Tags:** #process-lifecycle #filesystem

## Description

On startup, the sidecar guarantees exactly one instance survives, however
many were already running and however close together they were launched. It
terminates every other `go-waybar-sidecar` process it can find, then takes an
exclusive lock so a concurrent launch can't slip past the check before it.
The newest instance always wins (unchanged from the original design): the
incumbent is terminated, not the newcomer.

This replaces the original pidfile-only takeover, which only ever knew about
one PID (whatever `savePID` last wrote) and took no lock — so it neither
noticed nor stopped a second, third, etc. already-running instance, and two
instances launched close together could both pass the check before either had
written the pidfile.

## Implementation

`pid.go`:
- `sidecarPIDs() []int`: scans `/proc` for other `go-waybar-sidecar`
  processes (excluding `os.Getpid()`) via `isRealProc(pid,
  sidecarExeSuffix)`. **Not** `pgrep`-based: `go-waybar-sidecar` is 17
  characters, and Linux truncates `comm` to 15, so `pgrep -x` can never match
  it (verified: it errors out rather than silently failing); `pgrep -f`
  matches unrelated processes whose command line merely mentions the name.
  A `/proc/<pid>/exe` scan is exact.
- `isRealProc(pid int, exeSuffix string) bool`: live (non-zombie, via
  `/proc/<pid>/status`) and `/proc/<pid>/exe` ends in `exeSuffix`. Shared
  with `waybarPIDs()` as `isRealProc(pid, "/waybar")`.
- `terminateSidecars(pids []int)`: sends `SIGTERM` to each pid, polls
  (`sidecarStopPoll`, 100ms) for up to `sidecarStopGrace` (5s) for all to
  exit, then sends `SIGKILL` to any still alive (logged at warn). Uses
  `killFn` (`waybar.go`'s existing test seam), not `syscall.Kill` directly.
- `acquireInstanceLock() (locked bool, err error)`: opens
  `<pidDir>/waybar-sidecar.lock` and takes a non-blocking exclusive
  `flock`. `EWOULDBLOCK` (lock held by another process) is reported as
  `locked=false, err=nil`, not a failure. Held for the process's lifetime in
  the package var `instanceLock` (released automatically on exit, including
  a hard kill, since it's a kernel-held file lock).
- `ensureSingleInstance()`:
  1. `sidecarPIDs()` → `terminateSidecars()` — always run, regardless of lock
     state, since a stray may predate the lock file or have crashed oddly.
  2. `acquireInstanceLock()`. If busy, retry every `sidecarStopPoll` for up
     to `sidecarStopGrace` (the terminated incumbent is exiting and will
     release it shortly). If still busy after that, log a warning and
     proceed unlocked rather than hang forever.

## Configuration

- `pidDir` = `~/.local/state/waybar`.
- `sidecarStopGrace` (5s) / `sidecarStopPoll` (100ms): internal timing vars,
  not user-configurable (mirrors `usr2SettleDelay` etc. in `waybar.go`).

## Testing

### Human Testing

- Start one instance, then start a second: the first logs a shutdown message
  and exits; the second logs the takeover and keeps running.
- Start three instances in quick succession (e.g. `for i in 1 2 3; do
  go-waybar-sidecar & done`), wait, then check `/proc`: exactly one
  `go-waybar-sidecar` process remains.
- `kill -SIGSTOP` a running instance, then start a new one: the new one logs
  the SIGTERM timeout, then a SIGKILL, and the stopped process is gone
  (`kill -0 <pid>` fails).
- `--daemon` mode: after the fork, exactly one process remains, with no
  spurious "killed stray" log entries about itself.

### Unit Testing

- `isRealProc`: current process matches its own real exe suffix; does not
  match an unrelated suffix (e.g. `/waybar`); a nonexistent PID returns
  false without panicking.
- `sidecarPIDs`: never includes the calling process's own PID.
- `acquireInstanceLock`: first call on a fresh lock file succeeds; a second,
  independent acquisition attempt on the same path (separate file
  descriptor) reports `locked=false` with no error.
- `terminateSidecars`: with a stubbed `killFn` that reports a pid as gone
  after the first liveness probe, only `SIGTERM` is sent (no `SIGKILL`).
  With a stubbed `killFn` that always reports the pid alive, `SIGTERM` is
  sent, then — after the grace window — `SIGKILL`, and a warning is logged.
- `ensureSingleInstance`: with a stubbed `sidecarPIDsFn` returning no PIDs,
  no signals are sent and the lock is acquired. With a stubbed
  `sidecarPIDsFn` returning multiple PIDs, **all** of them receive
  `SIGTERM` (this is the regression case for the original one-PID-only
  bug). With the lock pre-held by another descriptor and no stray PIDs, it
  logs a warning and returns instead of hanging.

### Integration Testing

- Launch the binary three times in immediate succession against a temp
  `pidDir`/config: assert exactly one process remains alive after startup
  settles, and it holds the lock file.
- Launch a second instance while the first ignores `SIGTERM` (stopped via
  `SIGSTOP`): assert the second instance still starts, and the first is
  gone (via `SIGKILL`) rather than merely warned about.

## Status

Implemented.
