# GWS-0007. Waybar cold start fallback

## Description

If Waybar is not running, or it dies after a `SIGUSR2` reload (GWS-0006), the
sidecar kills any stray Waybar process and starts a fresh one, detached from
the sidecar's own session.

## Implementation

`waybar.go` `reloadWaybar()`, fallback path after the SIGUSR2 reload attempt
fails or finds no running instance:
- Logs "waybar not running, performing cold start".
- Runs `pkill -x waybar` to clear out any stray/unresponsive process, then
  sleeps 200ms.
- Starts `waybar` via `exec.Command("waybar")` with `SysProcAttr{Setsid:
  true}` so it survives the sidecar's own exit/restart and is not killed by
  signals sent to the sidecar's process group.
- Polls up to 4 times, 500ms apart (2s total), calling `waybarPID()`. On the
  first successful detection:
  - Logs "waybar started" with the new PID.
  - Saves the PID via `savePID("waybar", alive)` (GWS-0009).
  - Returns.
- If still not detected after 2s, logs an error "waybar failed to start
  within 2s".

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
  containing a stub `waybar` binary that stays alive: assert the stub starts,
  `waybarPID()` returns its PID, and it's saved via `savePID("waybar", ...)`.
- With `PATH` containing no `waybar` binary: assert `reloadWaybar()` logs
  "failed to start waybar" and returns without panicking.
- With a stub `waybar` that never reaches a running state within 2s: assert
  `reloadWaybar()` logs "waybar failed to start within 2s".

## Status

Implemented.
