# GWS-0009. PID file tracking

## Description

The sidecar records the PIDs of itself and the Waybar process it manages in a
shared state directory, so they can be inspected externally and so
GWS-0008's takeover logic has a PID to check.

## Implementation

`pid.go` `savePID(name, pid)`:
- Ensures `pidDir` (`~/.local/state/waybar`) exists (`0o755`).
- Writes `<pid>\n` to `<pidDir>/<name>.pid` (`0o644`).
- Logs at debug on success, at error (without exiting) on failure to create
  the directory or write the file.

Called with two names:
- `"waybar-sidecar"` — written once at startup in `run()` with the sidecar's
  own `os.Getpid()`.
- `"waybar"` — written by `reloadWaybar()` (`waybar.go`) whenever a Waybar
  PID is confirmed, whether via SIGUSR2 reload (GWS-0006) or cold start
  (GWS-0007).

## Configuration

- `pidDir` = `~/.local/state/waybar`.
- Files: `waybar-sidecar.pid`, `waybar.pid`.

## Testing

### Human Testing

- Start the sidecar and confirm `~/.local/state/waybar/waybar-sidecar.pid`
  contains its PID (matches `pgrep -f go-waybar-sidecar`).
- Confirm `~/.local/state/waybar/waybar.pid` contains the running Waybar
  process's PID after the first generation.

### Unit Testing

- `savePID()` with a writable temp `pidDir`: assert
  `<pidDir>/<name>.pid` contains `"<pid>\n"`.
- `savePID()` with a `pidDir` that cannot be created (e.g. path under a
  read-only or non-existent parent without permissions): assert it logs an
  error and does not panic.

### Integration Testing

- Run `run()` against a temp `pidDir`/config and assert
  `waybar-sidecar.pid` is written with the current process's PID before
  `select{}` blocks.

## Status

Implemented.
