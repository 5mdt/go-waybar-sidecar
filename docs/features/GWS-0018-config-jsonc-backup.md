# GWS-0018. Backup of config.jsonc before regeneration (.bak)

**Tags:** #config-generation #config

## Description

Before writing a new `config.jsonc`, the sidecar copies the previous file to `config.jsonc.bak` so a known-good copy is available if the new one is broken. Output files are written atomically, so `config.jsonc` never disappears or ends up truncated. Unchanged output is not rewritten and does not touch `.bak`. `.bak` and temp files are excluded from the file watcher so they don't trigger reload loops.

## Implementation

`src/generator.go` `writeJSON()`:
- Encodes the JSON to memory. If the bytes equal the current `outJSON` after its leading `//` header lines, returns without writing.
- Otherwise calls `backupFile(outJSON)`, then `writeFileAtomic(outJSON, data)`.
- `backupFile(path)` copies `path` to `path + ".bak"` if `path` exists; if `path` does not exist, it is a no-op.
- `writeFileAtomic(path, data)` creates the parent dir, writes a temp file `<path>.tmp*` in the same dir, and renames it over `path`. Also used for menu XML, job output files and `ensureSettings`.

`.bak` and `.tmp*` files are not whitelisted by `isWatchedFile()` (GWS-0005), so they never trigger reloads.

## Configuration

- Backup path: `<outJSON>.bak` (e.g. `~/.config/waybar/config.jsonc.bak`). Not configurable.

## Testing

### Human Testing

- Trigger two regenerations (e.g. edit `config.yaml` twice): confirm `config.jsonc.bak` exists and contains the content written by the first regeneration.
- Confirm no "change detected" log appears for `config.jsonc.bak`.

### Unit Testing

- `backupFile()` on an existing file: assert `<path>.bak` has the same content and the original path still exists.
- `writeFileAtomic()`: assert content written, no `*.tmp*` file left behind.
- `writeJSON()` twice with identical input: assert no `.bak` and the output file is not rewritten (mtime unchanged), even though the header timestamp would differ.
- `backupFile()` on a non-existent file: assert no error and no `.bak` file is created.
- `isWatchedFile()`: assert false for `config.jsonc.bak` (covered by `TestIsWatchedFile` in `src/watcher_test.go`).

### Integration Testing

- Call `writeJSON()` twice with different `waybar` configs: assert `outJSON + ".bak"` contains the first run's output and `outJSON` contains the second.

## Status

Implemented.
