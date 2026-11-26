# GWS-0007. Waybar cold start fallback

**Tags:** #reload #process-lifecycle

## Description

If Waybar is not running, or it dies after a `SIGUSR2` reload (GWS-0006), the sidecar stops any process it was tracking and starts a fresh one as a supervised child (GWS-0027), detached from the sidecar's own session.

## Implementation

`waybar.go` `reloadWaybar()`, fallback path after the SIGUSR2 reload attempt fails or finds no running instance:
- Holds a package-level mutex (`reloadMu`) for its entire body, so only one reload (SIGUSR2 + cold start) runs at a time. Concurrent callers block and then observe the already-restarted waybar instead of racing their own cold start. Why: concurrent cold starts would `stop`/`start` each other's freshly-spawned process and produce a spurious failure log.
- Logs "waybar not running, performing cold start" at info, only when no tracked waybar is alive. A failed or fatal `SIGUSR2` logs its own warning (GWS-0006) instead.
- Calls `waybar.stop()` then `waybar.start()` (GWS-0027), which runs `waybar` in its own session (`Setsid`).
- Waits `coldStartSettleDelay` (500ms), then checks `waybar.alive()`. If alive, logs "waybar started" with the PID and returns.
- Otherwise logs an error, "waybar failed to start".

## Configuration

- Requires a `waybar` binary on `PATH`.
- Cold-start settle delay: 500ms (`coldStartSettleDelay`, hardcoded).

## Testing

### Human Testing

- Kill Waybar manually, then trigger a config change: confirm the sidecar logs "waybar not running, performing cold start" and a new Waybar process starts.
- Stop Waybar with `SIGSTOP` so it can't handle `SIGUSR2`/exit, trigger a reload, and confirm the sidecar's `stop()` clears it (escalating to `SIGKILL` after `stopGraceDelay` since a stopped process can't act on `SIGTERM`) and a fresh instance starts.

### Unit Testing

See GWS-0027 for `waybarProc` unit tests (`start`, `alive`, `stop`).

### Integration Testing

- With no `waybar` process running, run `reloadWaybar()` against a `PATH` containing a stub `waybar` binary that stays alive: assert the stub starts and `waybar.alive()` is `true` (no error logged).
- With `PATH` containing no `waybar` binary: assert `reloadWaybar()` logs "failed to start waybar" and returns without panicking.
- Two concurrent `reloadWaybar()` calls: assert they do not interleave, and that no failure is logged once waybar is running (run with `-race`).

## Status

Implemented.
