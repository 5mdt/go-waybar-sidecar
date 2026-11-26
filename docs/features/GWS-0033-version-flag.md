# GWS-0033. Version flag (--version)

**Tags:** #cli

## Description

`--version` prints the build version and exits, so users and bug reports can identify the binary.

## Implementation

`src/main.go`:
- `newFlagSet()` registers a bool flag `version` (not a settings row: no env var, no config key).
- `versionString()` returns `go-waybar-sidecar <version>`; `version` defaults to `dev` and is set by `make build` via `-ldflags -X main.version` from `git describe`.
- `main()` checks the flag right after `fs.Parse`, before any settings or config loading, prints `versionString()` to stdout and returns (exit 0).

## Configuration

- `--version`: print the version and exit.

## Testing

### Human Testing

- Run `make build && build/go-waybar-sidecar --version`: prints `go-waybar-sidecar <git describe>`.
- Run `go-waybar-sidecar --version` from a plain `go build`: prints `go-waybar-sidecar dev`.

### Unit Testing

- `newFlagSet()` parses `--version` as `true`; the default is `false`.
- `versionString()` with `version = "v1.2.3"` returns `go-waybar-sidecar v1.2.3`.

### Integration Testing

- None.

## Status

Implemented.
