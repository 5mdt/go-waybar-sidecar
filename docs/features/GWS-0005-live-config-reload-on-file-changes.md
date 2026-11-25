# GWS-0005. Live config reload on file changes

**Tags:** #config #reload

## Description

The sidecar watches the config file and the Waybar config directory for
changes and regenerates `config.jsonc` and the per-menu XML files (see
GWS-0015) automatically, debounced to avoid redundant rebuilds during rapid
edits.

## Implementation

`watcher.go` `watch(trigger func()) error`, run as a goroutine from `run()`:
- Creates an `fsnotify.Watcher` and adds watches for `configPath` and
  `waybarDir` via `addWatch()`. `addWatch()` watches a file directly, or walks
  a directory and watches every subdirectory (fsnotify is non-recursive).
- On each event:
  - Ignores ops other than Write/Create/Remove/Rename (`isRelevantOp()`).
  - Ignores events on the sidecar's own outputs (`outJSON`, and any file
    under `menuDir`), to avoid reacting to its own writes.
  - Ignores files not in the per-directory whitelist via `isWatchedFile()`:
    files under `gwsDir` must have extension `.yaml` or `.yml`; files under
    `waybarDir` must have extension `.json`, `.jsonc`, or `.css`.
    Editor artifacts (`.swp`, `~`, `.bak`, all-numeric names, etc.) are
    implicitly excluded as their extensions are not whitelisted.
  - Otherwise logs the change and calls `trigger`.
- `debounce(d, f) func()` returns the trigger built in `run()`: each call
  restarts a 200ms timer, and when it fires `f` signals the reload loop
  (below). The power-profile watcher (GWS-0029) shares the same trigger.
- `run()` ends in one reload loop that owns the `Config`: for each signal it
  runs `cfg = reload(cfg)`, so reloads are sequential and nothing else touches
  the config. `reload(cfg Config) Config` calls `loadConfig()` and
  `generate()`; if `loadConfig()` fails, the previous config is returned
  (see GWS-0017).
- Directories created after startup under a watched root are added via
  `addWatch()` (`handleCreate()`). A root missing at startup is tracked in
  `pending`; its nearest existing ancestor is watched, and the root is added
  once it appears (a created directory that is an ancestor of a pending root
  is watched too). A new app dir triggers a reload.
- Watcher errors are logged at warn and do not stop the loop.

## Configuration

- Watched paths: `gwsDir` (`~/.config/go-waybar-sidecar`, recursively) and
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
- `isWatchedFile()` under `waybarDir`: assert true for `config.jsonc`,
  `style.css`, `foo.json`; assert false for `config.yaml`, `config.jsonc.bak`,
  `4913`, `style.css~`.
- `isWatchedFile()` under `gwsDir`: assert true for `config.yaml`, `foo.yml`;
  assert false for `config.json`, `.config.yaml.swp`, `config.yaml~`.

- `handleCreate()`: a new subdirectory and a pending root get watched
  (`w.WatchList()`); `nearestExisting()` returns the closest existing
  ancestor; `isWatchedFile()` is false for `.json` outside `waybarDir`.
- `debounce()`: three rapid calls invoke the callback once.

### Integration Testing

- Start `watch()` against a temp `configPath`/`waybarDir`, modify
  `configPath`, and assert the trigger fires within a small margin.
- Modify `outJSON` or a file under `menuDir` directly and assert no
  regeneration is triggered.

## Status

Implemented.
