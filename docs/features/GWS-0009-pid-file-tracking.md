# GWS-0009. PID file tracking

**Tags:** #process-lifecycle

## Description

The sidecar records its own PID in a shared state directory. The pidfile is
informational (external tooling, human inspection) — since GWS-0008's rework
it is no longer read by the takeover logic itself, which discovers other
instances directly via `/proc`.

## Implementation

`pid.go` `savePID(name, pid)`:
- Ensures `pidDir` (`~/.local/state/waybar`) exists (`0o755`).
- Writes `<pid>\n` to `<pidDir>/<name>.pid` (`0o644`).
- Logs at debug on success, at error (without exiting) on failure to create
  the directory or write the file.

Called with one name:
- `"waybar-sidecar"` - written once at startup in `run()` with the sidecar's
  own `os.Getpid()`.

The Waybar process PID is not stored — `waybarPID()` discovers it fresh via
`pgrep -x waybar` on every reload (GWS-0006, GWS-0007).

## Configuration

- `pidDir` = `~/.local/state/waybar`.
- Files: `waybar-sidecar.pid` (informational), `waybar-sidecar.lock` (the
  takeover lock, see GWS-0008).

## Testing

### Human Testing

- Start the sidecar and confirm `~/.local/state/waybar/waybar-sidecar.pid`
  contains its PID (matches `pgrep -f go-waybar-sidecar`).

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
