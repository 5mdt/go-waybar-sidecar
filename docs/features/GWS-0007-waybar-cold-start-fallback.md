# GWS-0007. Waybar cold start fallback

**Tags:** #reload #process-lifecycle

## Description

If Waybar is not running, or it dies after a `SIGUSR2` reload (GWS-0006), the
sidecar kills any stray Waybar process and starts a fresh one, detached from
the sidecar's own session.

## Implementation

`waybar.go` `reloadWaybar()`, fallback path after the SIGUSR2 reload attempt
fails or finds no running instance:
- Holds a package-level mutex for its entire body, so only one reload (reap +
  SIGUSR2 + cold start + poll) runs at a time. Concurrent callers block and
  then observe the already-restarted waybar instead of racing their own cold
  start. Why: concurrent cold starts `pkill` each other's freshly-spawned
  process and produce a spurious "waybar failed to start within 2s".
- Logs "waybar not running, performing cold start".
- Runs `pkill -x waybar` to clear out any stray/unresponsive process, then
  sleeps 200ms.
- Starts `waybar` via `exec.Command("waybar")` with `SysProcAttr{Setsid:
  true}` so it survives the sidecar's own exit/restart and is not killed by
  signals sent to the sidecar's process group.
- Polls up to 4 times, 500ms apart (2s total), calling `waybarPID()`. On the
  first successful detection, logs "waybar started" with the PID and returns.
- If the poll detects nothing, performs one final `waybarPID()` check. A
  waybar process may already be running under a different PID (e.g. one that
  was started externally, or whose PID `pgrep` returned too slowly). If found,
  logs "waybar started" and returns.
- Only if still not detected logs an error "waybar failed to start within 2s".

## Configuration

- Requires a `waybar` binary on `PATH`.
- Cold-start poll: 4 attempts × 500ms (hardcoded).

## Testing

### Human Testing

- Kill Waybar manually (`pkill -x waybar`), then trigger a config change:
  confirm the sidecar logs "waybar not running, performing cold start" and a
  new Waybar process starts within ~2s.
- Stop Waybar with `SIGSTOP` so it can't handle `SIGUSR2`/exit, trigger a
  reload, and confirm the sidecar's `pkill -x waybar` clears it and a fresh
  instance starts.

### Unit Testing

- `waybarPID()` immediately after a stub `waybar` process starts but before
  it's fully initialized: assert it eventually returns a non-zero PID within
  the polling window once the stub is ready.

### Integration Testing

- With no `waybar` process running, run `reloadWaybar()` against a `PATH`
  containing a stub `waybar` binary that stays alive: assert the stub starts
  and `waybarPID()` returns its PID within the poll window (no error logged).
- With a stub `waybar` detectable only after the poll window (appears just
  before the final check): assert `reloadWaybar()` logs "waybar started" and
  does not log "waybar failed to start within 2s".
- With `PATH` containing no `waybar` binary: assert `reloadWaybar()` logs
  "failed to start waybar" and returns without panicking.
- With a stub `waybar` that never reaches a running state: assert
  `reloadWaybar()` logs "waybar failed to start within 2s".
- Two concurrent `reloadWaybar()` calls: assert they do not interleave, and
  that no "waybar failed to start within 2s" is logged once waybar is
  detectable (run with `-race`).

## Status

Implemented.
