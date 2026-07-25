# GWS-0021. Symlink, hardlink, and mount handling

**Tags:** #config #filesystem

## Description

How the app behaves when config files or directories are symlinks, hardlinks,
or bind mounts.

## Implementation

Paths are resolved to their real location at startup via `resolvedPath()`
(`config.go`), so watches use real paths even when a path is a symlink.

- **Symlinked directories** (`~/.config/waybar`, `~/.config/go-waybar-sidecar`):
  followed — watched by their resolved real path.
- **Symlinked files** (e.g. `config.yaml`, `style.css`): followed. `watch()`
  (`watcher.go`) also watches each symlink target's parent directory
  (`symlinkTargets()`), because inotify on a directory only reports events for
  its own entries, not a symlink target living elsewhere.
- **Hardlinks / bind mounts**: behave as regular files; the kernel resolves
  them. A bind mount must be present when the watcher starts.
- **Symlinked subdirectories**: followed — watched recursively under their
  logical (pre-symlink) path, via `collectWatchDirs()`; a cycle (a symlink
  pointing back at an ancestor) is broken by tracking each directory's resolved
  real path.

Symlinks are resolved once at startup; re-pointing one at runtime needs a
restart. Following a symlinked subdirectory watches its whole subtree, so a
symlink into a very large tree adds an inotify watch per directory in it.

## Configuration

None.

## Testing

### Human Testing

- Symlink `~/.config/waybar` or `~/.config/go-waybar-sidecar` to a real dir,
  edit a file inside: change is detected.
- Symlink `style.css`/`config.yaml` to a target in another dir, edit the
  target: reload fires.

### Unit Testing

- `TestSymlinkTargets`, `TestAcceptEvent`, `TestCollectWatchDirs`
  (`watcher_test.go`).

## Status

Implemented.
