# GWS-0027. Waybar process supervision

**Tags:** #process-lifecycle #reload

## Description

The sidecar owns the waybar process it starts as a supervised child, instead
of rediscovering it via `pgrep` on every liveness check. Liveness is "is my
child still alive" (an in-memory channel check), not a `/proc` heuristic, so
there is no window where a dead child is misreported as running or a live one
misreported as dead. The child is `Wait()`-ed so it never becomes a zombie,
stopped alongside the sidecar so it never becomes an orphan, and restarted
automatically (with a rate limit) if it exits without the sidecar asking it
to.

This replaces the previous `pgrep -x waybar` / `/proc/<pid>/stat` based
liveness checks (`waybarPID`, `procStartTime`) used by GWS-0006, GWS-0007, and
GWS-0022, which had a real bug: `waybarPID()` inspected only the first line of
`pgrep -x waybar`, and an un-`Wait()`-ed zombie (lower PID, sorts first) made
that check permanently report "not running" even while a live waybar existed
- causing every reload to needlessly `pkill` the live instance and cold-start
a replacement, which itself became a new zombie.

## Implementation

`waybar.go`:
- `waybarProc` holds the current `*exec.Cmd`, a `done` channel closed when
  `Wait()` returns, a `generation` counter, and an `expectExit` flag - all
  guarded by a mutex. Package-level `var waybar waybarProc` (mirrors `var jobs
  jobRunner` in `cron.go`).
- `start() error`: `exec.Command("waybar")` with the existing
  `SysProcAttr{Setsid: true}` (own session, shielded from terminal signals sent to the sidecar's group; the sidecar stops it on shutdown), then
  `Start()`. A goroutine calls `cmd.Wait()` - this reaps the child, so it
  never becomes a zombie - then closes `done` and calls `onExit`.
- `alive() bool`: `true` iff a process was started and its `done` channel is
  not yet closed. No `pgrep`, no `/proc`, no race window.
- `pid() int`: the current child's PID, or `0`.
- `stop()`: sets `expectExit` (so the exit this causes is not treated as a
  crash), sends `SIGTERM` to the process group (`-pid`, since `Setsid` makes
  pid == pgid), waits on `done` up to `stopGraceDelay`, escalating to
  `SIGKILL` if it doesn't exit in time. Idempotent (no-op if nothing is
  running) and safe to call repeatedly.
- `onExit(generation int)`: runs in the `Wait()` goroutine. If a newer `start()`
  has since occurred (`generation` stale) or the exit was expected (`stop()`
  was called), it does nothing - the caller that triggered the stop is already
  handling what comes next. Otherwise the process died on its own, and it
  calls `scheduleAutoRestart`.
- `scheduleAutoRestart()`: records the restart under a rate limit
  (`autoRestartMax` restarts per `autoRestartWindow`, both vars); if the
  limit is exceeded, logs an error and gives up. Otherwise logs a warning,
  takes `reloadMu` so it can't race a manual `reloadWaybar()`, and calls
  `start()` unless a live waybar is already tracked (a reload replaced it).
- `reloadWaybar()` (called at the end of `generate()`, see GWS-0005/GWS-0006):
  if `waybar.alive()`, sends `SIGUSR2` (via `killFn`) to `waybar.pid()` and, after
  `usr2SettleDelay`, checks `alive()` again - success returns, failure falls
  through. Otherwise (or on fallthrough): `waybar.stop()` then `waybar.start()`;
  after `coldStartSettleDelay`, `alive()` decides between "waybar started" and
  "waybar failed to start". No poll loop - a single settle-and-check.
- `waybarPIDs()` (unchanged) are kept for
  `sweepStrayWaybars()`, called once from `main.go` `run()` at startup: the
  sidecar owns nothing yet, so every `waybar` process found is a stray from a
  previous session (crash, or a takeover, see GWS-0008) and gets `SIGTERM`.
  This replaces the old `reapDuplicateWaybars()`'s start-time arbitration -
  once the sidecar owns its own child, there is nothing left to arbitrate.
- `main.go`'s `SIGINT`/`SIGTERM` handler calls `waybar.stop()` before
  `os.Exit(0)` (GWS-0012), so waybar never outlives the sidecar - including
  during the `ensureSingleInstance()` takeover (GWS-0008), which is what
  produced the "2 instances" symptom this feature was written to fix: the
  outgoing instance's waybar is now stopped as part of its own shutdown,
  instead of being orphaned for the incoming instance to fight over.

## Configuration

- `usr2SettleDelay`, `coldStartSettleDelay`, `stopGraceDelay`: internal timing
  vars (500ms/500ms/2s), not user-configurable.
- `autoRestartMax` / `autoRestartWindow`: auto-restart rate limit (5 restarts
  per minute), not user-configurable.

## Testing

### Human Testing

- Edit `config.yaml` repeatedly (5+ times in a row): `pgrep -a -x waybar`
  stays at exactly one live process with zero `<defunct>` entries throughout.
- `pkill -x waybar`: confirm a replacement starts on its own within a couple
  seconds (auto-restart), logged as "waybar exited unexpectedly, restarting".
- Restart the sidecar: confirm the old waybar exits with it (no orphan
  reparented to `systemd --user` or similar) and exactly one new waybar comes
  up.
- `kill -SIGSTOP` then repeatedly `pkill -x waybar` waybar in a tight loop
  (more than `autoRestartMax` times within `autoRestartWindow`): confirm the
  sidecar logs "waybar crashed repeatedly, giving up on auto-restart" and
  stops trying.

### Unit Testing

- `start()` then `alive()`: `true`; after the stubbed process exits (or after
  `stop()`), `alive()`: `false`, and `cmd.ProcessState != nil` (proves
  `Wait()` ran - the zombie regression test).
- `stop()` terminates a running child and is a no-op when called with nothing
  running, and when called twice in a row.
- `onExit` after a deliberate `stop()`: no auto-restart triggered.
- `onExit` after an unprompted exit: `scheduleAutoRestart` runs and a new
  process starts.
- `scheduleAutoRestart` with a live tracked waybar: no new process started.
- `scheduleAutoRestart` beyond `autoRestartMax` within `autoRestartWindow`:
  no further restart attempted, error logged.
- `reloadWaybar()` with a live child: SIGUSR2 path taken, no new process
  spawned.
- `reloadWaybar()` with a dead/no child: cold start spawns exactly one new
  process; "waybar started" logged.
- `reloadWaybar()` with no `waybar` binary on `PATH`: "failed to start
  waybar" logged, no panic.
- `sweepStrayWaybars()`: every PID from a stubbed `waybarPIDsFn` receives
  `SIGTERM`.

### Integration Testing

- With a stub `waybar` binary on `PATH`: `reloadWaybar()` cold-starts it,
  `waybar.alive()` is `true`, and killing the stub process directly (not via
  `stop()`) results in an automatic restart.
- Two concurrent `reloadWaybar()` calls (`-race`): serialized by `reloadMu`,
  no interleaved pkill/start, no spurious failure log.

## Status

Implemented.
