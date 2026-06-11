# GWS-0010. Daemon mode

## Description

The `--daemon` flag re-launches the sidecar in the background, detached from
the invoking terminal, and lets the foreground process exit immediately.

## Implementation

`main.go`:
- `main()` parses `--daemon`. If set, calls `forkDetach()` and returns
  immediately (does not call `run()`).
- `forkDetach()`:
  - Rebuilds `os.Args[1:]` with `--daemon`/`-daemon` stripped, so the child
    doesn't re-fork.
  - Runs `os.Args[0]` (the same binary) with those args via `exec.Command`.
  - Sets `SysProcAttr{Setsid: true}` to detach from the controlling terminal
    and `Stdin`/`Stdout`/`Stderr` to `nil` (closed), so the child's stdio is
    not tied to the parent's.
  - Starts the child and logs its PID ("forked to background"); does not
    wait for it.
- The child process runs `main()` again without `--daemon`, so it proceeds to
  `run()` normally. Since stdio is closed, `--log-output` should be set to a
  file path (or `stderr`/redirected) for logs to be visible (GWS-0011).

## Configuration

- Flag: `--daemon` (bool, default `false`).
- Combine with `--log-output <file>` to retain logs, since stdio is closed.

## Testing

### Human Testing

- Run `waybar-sidecar --daemon --log-output ~/.local/state/waybar/sidecar.log`
  from a terminal: confirm the command returns immediately, a child process
  is running (`pgrep -f go-waybar-sidecar`), and the log file is being
  written.
- Confirm closing the terminal does not terminate the daemonized child
  (`Setsid` detachment).

### Unit Testing

- `forkDetach()` argument rebuilding: assert `--daemon`/`-daemon` is removed
  from the args passed to the child while other flags are preserved.

### Integration Testing

- Run the binary with `--daemon --log-output <tmpfile>` and a temp `HOME`:
  assert the parent process exits quickly, a new `go-waybar-sidecar` process
  remains running afterward, and `<tmpfile>` contains the "starting waybar
  sidecar" log line.

## Status

Implemented.
