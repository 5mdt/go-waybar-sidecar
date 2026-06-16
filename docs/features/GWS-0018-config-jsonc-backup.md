# GWS-0018. Backup of config.jsonc before regeneration (.bak)

**Tags:** #config-generation #config

## Description

Before writing a new `config.jsonc`, the sidecar preserves the previous file
as `config.jsonc.bak` so a known-good copy is available if the new one is
broken. `.bak` files are excluded from the file watcher so they don't trigger
reload loops.

## Implementation

`generator.go` `writeJSON()`:
- Calls `backupFile(outJSON)` before `os.Create(outJSON)`.
- `backupFile(path)` renames `path` to `path + ".bak"` if `path` exists; if
  `path` does not exist, it is a no-op.

`watcher.go` `isWatchedFile()`:
- `.bak` files are not whitelisted (they are not `.json`/`.jsonc`/`.css`),
  so they are implicitly excluded from triggering reloads.

## Configuration

- Backup path: `<outJSON>.bak` (e.g. `~/.config/waybar/config.jsonc.bak`).
  Not configurable.

## Testing

### Human Testing

- Trigger two regenerations (e.g. edit `config.yaml` twice): confirm
  `config.jsonc.bak` exists and contains the content written by the first
  regeneration.
- Confirm no "change detected" log appears for `config.jsonc.bak`.

### Unit Testing

- `backupFile()` on an existing file: assert it is renamed to `<path>.bak`
  and the original path no longer exists.
- `backupFile()` on a non-existent file: assert no error and no `.bak` file
  is created.
- `isWatchedFile()`: assert false for `config.jsonc.bak` (covered by `TestIsWatchedFile` in `watcher_test.go`).

### Integration Testing

- Call `writeJSON()` twice with different `waybar` configs: assert
  `outJSON + ".bak"` contains the first run's output and `outJSON` contains
  the second.

## Status

Implemented.
