# GWS-0027. Waybar process supervision

**Tags:** #process-lifecycle #reload

## Description

The sidecar owns the waybar process it starts as a supervised child. Liveness is an in-memory channel check, the child is `Wait()`-ed (never a zombie), stopped with the sidecar (never an orphan), and restarted automatically, with a rate limit, if it exits unprompted.

Why: process-table lookups (`pgrep`) misreported liveness when zombies existed.

## Implementation

`src/waybar.go`:
- `waybarProc` holds the current `*exec.Cmd`, a `done` channel closed when `Wait()` returns, a `generation` counter, and an `expectExit` flag - all guarded by a mutex. Package-level `var waybar waybarProc` (mirrors `var jobs jobRunner` in `src/cron.go`).
- `start() error`: `exec.Command("waybar")` with `SysProcAttr{Setsid: true}` (own session, shielded from terminal signals). A goroutine calls `cmd.Wait()`, then closes `done` and calls `onExit`.
- `alive() bool`: `true` iff a process was started and its `done` channel is not yet closed. No `pgrep`, no `/proc`, no race window.
- `pid() int`: the current child's PID, or `0`.
- `stop()`: sets `expectExit` (so the exit this causes is not treated as a crash), sends `SIGTERM` to the process group (`-pid`, since `Setsid` makes pid == pgid), waits on `done` up to `stopGraceDelay`, escalating to `SIGKILL` if it doesn't exit in time. Idempotent (no-op if nothing is running) and safe to call repeatedly.
- `onExit(generation int)`: runs in the `Wait()` goroutine. If a newer `start()` has since occurred (`generation` stale) or the exit was expected (`stop()` was called), it does nothing - the caller that triggered the stop is already handling what comes next. Otherwise the process died on its own, and it calls `scheduleAutoRestart`.
- `scheduleAutoRestart()`: records the restart under a rate limit (`autoRestartMax` restarts per `autoRestartWindow`, both vars); if the limit is exceeded, logs an error and gives up. Otherwise logs a warning, takes `reloadMu` so it can't race a manual `reloadWaybar()`, and calls `start()` unless a live waybar is already tracked (a reload replaced it).
- `reloadWaybar()` uses this API (GWS-0006, GWS-0007); startup sweeps strays (GWS-0022); shutdown calls `waybar.stop()` (GWS-0012).

## Configuration

- `usr2SettleDelay`, `coldStartSettleDelay`, `stopGraceDelay`: internal timing vars (500ms/500ms/2s), not user-configurable.
- `autoRestartMax` / `autoRestartWindow`: auto-restart rate limit (5 restarts per minute), not user-configurable.

## Testing

### Human Testing

- Edit `config.yaml` repeatedly (5+ times in a row): `pgrep -a -x waybar` stays at exactly one live process with zero `<defunct>` entries throughout.
- `pkill -x waybar`: confirm a replacement starts on its own within a couple seconds (auto-restart), logged as "waybar exited unexpectedly, restarting".
- Restart the sidecar: confirm the old waybar exits with it (no orphan reparented to `systemd --user` or similar) and exactly one new waybar comes up.
- `kill -SIGSTOP` then repeatedly `pkill -x waybar` waybar in a tight loop (more than `autoRestartMax` times within `autoRestartWindow`): confirm the sidecar logs "waybar crashed repeatedly, giving up on auto-restart" and stops trying.

### Unit Testing

- `start()` then `alive()`: `true`; after the stubbed process exits (or after `stop()`), `alive()`: `false`, and `cmd.ProcessState != nil` (proves `Wait()` ran - the zombie regression test).
- `stop()` terminates a running child and is a no-op when called with nothing running, and when called twice in a row.
- `onExit` after a deliberate `stop()`: no auto-restart triggered.
- `onExit` after an unprompted exit: `scheduleAutoRestart` runs and a new process starts.
- `scheduleAutoRestart` with a live tracked waybar: no new process started.
- `scheduleAutoRestart` beyond `autoRestartMax` within `autoRestartWindow`: no further restart attempted, error logged.
- `reloadWaybar()` with a live child: SIGUSR2 path taken, no new process spawned.
- `reloadWaybar()` with a dead/no child: cold start spawns exactly one new process; "waybar started" logged.
- `reloadWaybar()` with no `waybar` binary on `PATH`: "failed to start waybar" logged, no panic.
- `sweepStrayWaybars()`: every PID from a stubbed `waybarPIDsFn` receives `SIGTERM`.

### Integration Testing

- With a stub `waybar` binary on `PATH`: `reloadWaybar()` cold-starts it, `waybar.alive()` is `true`, and killing the stub process directly (not via `stop()`) results in an automatic restart.
- Two concurrent `reloadWaybar()` calls (`-race`): serialized by `reloadMu`, no interleaved pkill/start, no spurious failure log.

## Status

Implemented.
