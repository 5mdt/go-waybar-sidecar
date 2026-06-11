# GWS-0011. Configurable logging

## Description

Logging verbosity and destination are configurable via flags, using
structured text logs (`log/slog`).

## Implementation

`main.go`:
- `--log-level <level>` (default `"info"`): parsed by `parseLogLevel()`,
  which delegates to `slog.Level.UnmarshalText` — accepts `debug`, `info`,
  `warn`, `error` (case-insensitive). An invalid value logs an error and
  exits (`os.Exit(1)`) before any output destination is configured.
- `--log-output <dest>` (default `"stdout"`): parsed by `buildLogger(level,
  output)`:
  - `"stdout"` → `os.Stdout`.
  - `"stderr"` → `os.Stderr`.
  - any other value → treated as a file path; creates the parent directory
    (`0o755`) and opens the file for append (`O_CREATE|O_APPEND|O_WRONLY`,
    `0o644`).
  - Returns a `slog.Logger` using `slog.NewTextHandler` at the configured
    level.
- A bootstrap logger (`slog.NewTextHandler(os.Stderr, ...)`) is used before
  flags are fully processed (e.g. to report an invalid `--log-level`); the
  real logger replaces it once `buildLogger()` succeeds.
- In `--daemon` mode, the bootstrap stderr logger is used for the "forked to
  background" message, since the forking parent does not call
  `buildLogger()`.

## Configuration

- `--log-level`: `debug` | `info` | `warn` | `error` (default `info`).
- `--log-output`: `stdout` | `stderr` | `<file path>` (default `stdout`).

## Testing

### Human Testing

- Run with `--log-level debug`: confirm debug-level lines (e.g. "config
  already exists", "watching file") appear.
- Run with `--log-level info` (default): confirm debug lines are suppressed
  but info/warn/error lines appear.
- Run with `--log-output <path>` where `<path>`'s directory doesn't exist:
  confirm the directory is created and logs are written there.
- Run with `--log-level bogus`: confirm the process exits with an error
  before starting.

### Unit Testing

- `parseLogLevel("debug"|"info"|"warn"|"error")`: assert the corresponding
  `slog.Level` and no error.
- `parseLogLevel("bogus")`: assert a non-nil error.
- `buildLogger()` with `output == "stdout"` / `"stderr"`: assert the returned
  logger writes to the corresponding stream.
- `buildLogger()` with a file path under a non-existent directory: assert the
  directory is created and the file is opened for append.

### Integration Testing

- Run the binary with `--log-level debug --log-output <tmpfile>`: assert
  `<tmpfile>` contains debug-level entries.
- Run the binary with `--log-output <nested/dir/file.log>` where `nested/dir`
  doesn't exist: assert the directory and file are created.

## Status

Implemented.
