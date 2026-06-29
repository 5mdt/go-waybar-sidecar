# GWS-0025. Cron-like jobs runner

**Tags:** #config #jobs

## Description

The sidecar runs user-defined periodic jobs specified in the `jobs:` section
of `config.yaml`. Each job fires on either a fixed interval (`every:`) or a
standard 5-field cron expression (`schedule:`), runs a shell command, and
optionally writes the command's stdout to a file (e.g. for a Waybar
`custom/exec` module). Jobs are started on the first scheduled tick after the
sidecar starts. Editing jobs in `config.yaml` restarts the job set via the
live-reload path (see GWS-0005) without restarting the sidecar.

## Implementation

`cron.go` — the job scheduler and runner:
- `Job` struct defined in `config.go`: `name`, `every`, `schedule`,
  `command`, `output` yaml-tagged fields. `Jobs []Job` added to `Config` and
  `rawConfig`; set in `parseConfig()`.
- `expandHome(path string) string` in `config.go`: expands a leading `~` to
  `$HOME`.
- `parseSchedule(j Job) (schedule, error)`: validates exactly one of `every`
  or `schedule` is set; parses `every` with `time.ParseDuration` into
  `intervalSchedule{d}`; parses `schedule` with `cron.ParseStandard` from
  `github.com/robfig/cron/v3`. Returns an error if neither or both are set.
- `runJob(ctx context.Context, j Job, s schedule)`: computes
  `s.Next(jobNow())`, waits via `time.NewTimer`, then calls `jobExec(j.Command)`
  on each tick. Non-zero exit or error is logged at warn. If `j.Output` is
  set, trimmed stdout is written to `expandHome(j.Output)` (parent dir created
  with `os.MkdirAll`). Observes `ctx.Done()` while waiting and after each run
  to exit cleanly.
- `type jobRunner struct{ mu sync.Mutex; cancel context.CancelFunc; wg sync.WaitGroup }`:
  `restart(jobs []Job)` is guarded by `mu` (called from both startup and the
  watcher reload goroutine — data-race safe). It cancels the old context,
  drains in-flight runs with `wg.Wait()`, then starts fresh goroutines for
  all valid jobs.
- Overridable vars for testing (mirror `sendNotification` in `notify.go`):
  `var jobExec = func(command string) ([]byte, error)` and
  `var jobNow = time.Now`.
- Package-level `var jobs jobRunner`.

Wired in:
- `main.go` `run()`: calls `jobs.restart(cfg.Jobs)` after `generate(cfg)`.
- `watcher.go` `reload(cfg)`: calls `jobs.restart(cfg.Jobs)` after updating
  `*cfg`, so edited jobs take effect on live reload.

## Configuration

`config.yaml` `jobs:` list. Each entry:

```yaml
jobs:
  - name: sync-mail          # required; used in log messages
    every: 5m                # one of: time.Duration string (30s, 5m, 1h)
    command: "mbsync -a"     # required; run via sh -c
  - name: weather
    schedule: "*/15 * * * *" # one of: standard 5-field cron expression
    command: "curl -s 'wttr.in/?format=3'"
    output: ~/.cache/waybar/weather  # optional; stdout written here
```

Rules:
- Exactly one of `every` / `schedule` per job; both or neither is an error
  (job is logged and skipped, others continue).
- `name` and `command` are required.
- `output` is optional; `~` is expanded to `$HOME`.
- Jobs run concurrently with each other; a single job never overlaps itself.

## Testing

### Human Testing

- Add a job with `every: 2s`, `command: "date"`, `output: /tmp/gws-test`,
  run the sidecar with `--log-level debug`: confirm `/tmp/gws-test` updates
  every ~2s and debug logs show each run.
- While running, edit the job interval in `config.yaml` and save: confirm a
  reload log and the new schedule takes effect without restart.
- Add a job with both `every` and `schedule` set: confirm the sidecar logs an
  error for that job and skips it while all other jobs continue.
- Add a cron job `schedule: "* * * * *"` (every minute): confirm it fires on
  the next whole minute boundary.

### Unit Testing

- `parseSchedule`: valid interval, valid cron, both set → error, neither →
  error, bad duration string → error, bad cron expression → error.
- `intervalSchedule.Next`: returns `t + d`.
- `runJob`: stub `jobNow` + `jobExec`; assert command called on tick; assert
  `ctx` cancel stops the loop with no further calls; assert output file
  written with trimmed stdout when `j.Output` is set.
- `jobRunner.restart`: restarting stops the previous set (no further calls
  from old goroutines); new jobs run on new schedule.
- `config.go` `parseConfig` with a `jobs:` fixture: fields parsed correctly.
- `expandHome`: leading `~` replaced with `$HOME`; non-`~` path unchanged.

### Integration Testing

- Run the sidecar against a temp config with a short-interval job; assert the
  output file is created and updated within the expected window.
- Modify the config and trigger a reload; assert the old schedule stops and
  the new one starts.

## Status

Implemented.
