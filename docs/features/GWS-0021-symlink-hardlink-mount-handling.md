# GWS-0021. Symlink, hardlink, and mount handling

**Tags:** #config #filesystem

## Description

Documents how the application behaves when config files or directories are
symlinks, hardlinks, or bind mounts.

## Implementation

**Symlinks on top-level directories** (`~/.config/waybar`, `~/.config/go-waybar-sidecar`):
Handled. `resolvedPath()` (`config.go`) calls `filepath.EvalSymlinks` on both
the path and its parent at startup. All internal path variables (`waybarDir`,
`gwsDir`, `menuDir`, `outJSON`) are set to the resolved real path, so watches
and prefix comparisons are correct even if the directory itself is a symlink.

**Symlinked files inside watched directories**: Handled. All filesystem
operations use `os.Stat` (not `os.Lstat`), which follows symlinks. inotify
events are emitted on the symlink name within the watched directory.

**Hardlinks**: Handled. Hardlinks are indistinguishable from regular files at
the OS level.

**Bind mounts**: Handled. The kernel resolves them transparently; `fsnotify`
watches the inode within the mount namespace. Works as long as the mount is
present when the watcher starts.

**Symlinked subdirectories inside watched directories**: Not followed.
`filepath.Walk` (used in `addWatch()`) does not descend into symlinked
subdirectories. This is not a practical limitation since `gtk3-menus/` and
`go-waybar-sidecar/` are expected to be flat (no subdirectories).

**Watched directory re-symlinked at runtime**: Not detected. If `waybarDir` or
`gwsDir` itself is re-pointed to a different real directory after startup (e.g.
`ln -sfn new-dir ~/.config/waybar`), the watcher continues watching the
original resolved path. A restart is required to pick up the new target.

## Configuration

No configuration. Behavior is determined by how the OS exposes the paths at
startup.

## Testing

### Human Testing

- Point `~/.config/waybar` to a real directory via symlink; start sidecar and
  edit a file inside: confirm change is detected.
- Point `~/.config/go-waybar-sidecar` to a real directory via symlink; start
  sidecar and edit `config.yaml`: confirm reload fires.
- Use a hardlink for `config.yaml`: confirm changes are detected and config
  reloads.

### Unit Testing

N/A — behavior is a property of OS-level path resolution and `fsnotify`.

### Integration Testing

N/A — covered by human testing above.

## Status

Documented. No code changes required for the current flat directory structure.
