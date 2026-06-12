# GWS-0005. Live config reload on file changes

**Tags:** #config #reload

## Description

The sidecar watches the config file and the Waybar config directory for
changes and regenerates `config.jsonc` and the per-menu XML files (see
GWS-0015) automatically, debounced to avoid redundant rebuilds during rapid
edits.

## Implementation

`watcher.go` `watch(cfg)`, run as a goroutine from `run()`:
- Creates an `fsnotify.Watcher` and adds watches for `configPath` and
  `waybarDir` via `addWatch()`. `addWatch()` watches a file directly, or walks
  a directory and watches every subdirectory (fsnotify is non-recursive).
- On each event:
  - Ignores ops other than Write/Create/Remove/Rename (`isRelevantOp()`).
  - Ignores events on the sidecar's own outputs (`outJSON`, and any file
    under `menuDir`), to avoid reacting to its own writes.
  - Ignores editor artifact files via `isIgnoredFile()`: vim swap/backup
    files (`.swp`/`.swo`/`.swn`/`~` suffix) and vim's all-numeric write-test
    files (e.g. `4913`).
  - Otherwise logs the change and (re)starts a 200ms debounce timer. When the
    timer fires, it calls `loadConfig()` and `generate()` with the result
    stored back into `*cfg`.
- Watcher errors are logged at warn and do not stop the loop.

## Configuration

- Watched paths: `configPath` (`~/.config/waybar/config.yaml`) and
  `waybarDir` (`~/.config/waybar`, recursively).
- Debounce interval: 200ms (hardcoded).

## Testing

### Human Testing

- With the sidecar running, edit and save `config.yaml`: confirm a "change
  detected" log appears and `config.jsonc` and the per-menu XML files are
  regenerated within ~200ms.
- Save the file rapidly multiple times in succession: confirm only one
  regeneration occurs after the last save.
- Edit the file with vim (which creates swap files and a numeric write-test
  file): confirm no spurious "change detected" logs appear for those
  artifacts.
- Touch `outJSON` or a file under `menuDir` directly: confirm no
  regeneration loop occurs.

### Unit Testing

- `isRelevantOp()`: assert true for Write/Create/Remove/Rename and false for
  Chmod.
- `isIgnoredFile()`: assert true for `.foo.swp`, `.foo.swo`, `.foo.swn`,
  `foo~`, and `4913`; assert false for `config.yaml`.

### Integration Testing

- Start `watch()` against a temp `configPath`/`waybarDir`, modify
  `configPath`, and assert `generate()` runs (e.g. `outJSON`'s mtime updates)
  within the debounce window plus a small margin.
- Modify `outJSON` or a file under `menuDir` directly and assert no
  regeneration is triggered.

## Status

Implemented.
