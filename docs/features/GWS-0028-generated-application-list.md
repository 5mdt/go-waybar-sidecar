# GWS-0028. Generated application list

**Tags:** #menu #config-generation #filesystem

## Description

A menu item can pull in a flat, alphabetical list of installed applications, generated from `*.desktop` files, instead of listing each launcher by hand. The list is rescanned on every config load (startup and live reload), and the XDG application directories are watched so installing or removing an app refreshes the menu without a sidecar restart.

## Implementation

`desktop.go` - scanning and parsing:
- `appDirs() []string`: XDG application directories in precedence order - `$XDG_DATA_HOME/applications` (default `~/.local/share/applications`), then `<dir>/applications` for each entry of `$XDG_DATA_DIRS` (default `/usr/local/share:/usr/share`).
- `desktopEntry struct{ name, exec string }` and `parseDesktopEntry(b []byte) (desktopEntry, bool)`: hand-rolled line scanner over the `[Desktop Entry]` group only (other groups, e.g. `[Desktop Action ...]`, and localized keys like `Name[de]` are ignored). Returns `ok=false` when `NoDisplay=true`, `Hidden=true`, `Type` is not `Application`, or `Name`/`Exec` is missing. Also records `Terminal`, `OnlyShowIn`, `NotShowIn` and `TryExec`.
- `entryVisible(e desktopEntry, desktops []string) bool`: false when `OnlyShowIn` is set and shares no value with `desktops` (the `:`-separated `$XDG_CURRENT_DESKTOP`), when `NotShowIn` shares a value, or when `TryExec` is set and `exec.LookPath` fails.
- `stripFieldCodes(exec string) string`: removes Exec field codes (`%f %u %F %U %i %c %k %d %D %n %N %v %m`) and unescapes `%%` to `%` (a `%%` is never read as the start of a field code, so `%%f` yields `%f`).
- `scanApplications() []MenuItem` (package-level `var`, indirection for tests): walks `appDirs()` recursively for `*.desktop` files, parses each, dedupes by desktop file id (path relative to its app dir with `/` replaced by `-`) keeping the first directory in precedence order, skips entries failing `entryVisible`, and returns `MenuItem{Name: ..., Action: ...}` sorted alphabetically by name, case-insensitively. `Terminal=true` entries get the action `xdg-terminal-exec <cmd>`.

`config.go` - wiring into menus:
- `MenuItem.Include string` (`yaml:"include"`): when set, the item is replaced in place by the resolved alias's items instead of being rendered itself.
- `expandIncludes(items []MenuItem) []MenuItem`: recurses through `menu_definition` (including into `Children`), splicing in resolved items for any `Include` and dropping items whose alias is unknown (logged as a warning). The only alias currently registered is `applications:plain`, resolved via `scanApplications()`.
- Called from `loadConfig()` for every menu after `parseConfig` builds `cfg.Menus`.

`watcher.go`:
- `watch()` adds each of `appDirs()` to the fsnotify watch set (already tolerant of missing directories via `addWatch`).
- `isWatchedFile()` accepts `.desktop` files under any `appDirs()` path.

`Icon=` is carried into `MenuItem.Icon` (see GWS-0032).

## Configuration

```yaml
custom/Start:
  menu_definition:
    - name: Apps
      children:
        - include: applications:plain   # all *.desktop apps, alphabetical
    - name: Power
      children:
        - name: Shutdown
          action: "systemctl poweroff"
```

Rules:
- `include: applications:plain` may appear at the top level of `menu_definition` or inside any `children` list; it expands in place (siblings keep their order).
- An unknown `include` alias is dropped with a warning; the rest of the menu still generates.
- The alias is namespaced (`applications:<shape>`); the per-category variant is GWS-0031.

## Testing

### Human Testing

- Add `- include: applications:plain` under a menu item's `children:`, start the sidecar, and open the generated `gtk3-menus/*.xml`: one entry per installed application, alphabetical.
- Click an entry from the generated list in waybar: the app launches.
- With the sidecar running, copy a `.desktop` file into `~/.local/share/applications/`: the menu gains the entry without a restart. Remove it: the entry disappears.
- Set `include: applications:bogus`: sidecar logs a warning and keeps running; the rest of the menu is unaffected.

### Unit Testing

- `appDirs`: default paths; paths from `XDG_DATA_HOME`/`XDG_DATA_DIRS` env vars, in precedence order.
- `parseDesktopEntry`: extracts `Name`/`Exec`; skips `NoDisplay=true`, `Hidden=true`, `Type` != `Application`, missing `Name` or `Exec`; ignores keys outside `[Desktop Entry]` and localized `Name[xx]`.
- `parseDesktopEntry`: reads `Terminal`, `OnlyShowIn`, `NotShowIn`, `TryExec`.
- `entryVisible`: `OnlyShowIn` match/no match, `NotShowIn` match, `TryExec` missing binary hides, `TryExec=sh` keeps.
- `stripFieldCodes`: `%U`, `%f`, `%%`, `%%f`, and codes mid-string are handled.
- `scanApplications`: results sorted by name, case-insensitively; a `Terminal=true` entry's action is prefixed with `xdg-terminal-exec `; duplicate desktop file id across two dirs keeps the higher-precedence one; entries in subdirectories are found; a missing app dir is silently skipped.
- `expandIncludes`: splices at top level, inside `children`, and inside a nested submenu; drops unknown aliases without error; leaves menus with no `include` unchanged.

### Integration Testing

- `loadConfig()` against a config with `include: applications:plain` and a temp `XDG_DATA_HOME` containing sample `.desktop` files: `cfg.Menus` contains the expected expanded items.

## Status

Implemented.
