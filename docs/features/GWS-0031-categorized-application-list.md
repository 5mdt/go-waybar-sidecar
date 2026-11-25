# GWS-0031. Categorized application list

**Tags:** #menu #config-generation #filesystem

## Description

`include: applications:categories` expands to one submenu per freedesktop main
category, each holding the matching installed applications (GWS-0028 scan).
It is rescanned on every config load, like `applications:plain`.

## Implementation

`desktop.go`:
- `desktopEntry.categories []string`: `Categories=` split on `;`, trimmed,
  empty parts dropped. `scanApplications()` copies it to `MenuItem.Categories`.
- `mainCategories`: the main categories and their labels — AudioVideo
  ("Audio & Video"), Development, Education, Game ("Games"), Graphics,
  Network, Office, Science, Settings, System, Utility. Other values (`GTK`,
  `WebBrowser`, ...) are ignored.
- `categorizeApps(apps []MenuItem, mode string) []MenuItem`: buckets apps by
  label and returns `MenuItem{Name: label, Children: apps}` per non-empty
  bucket, sorted by label with "Other" last; apps keep name order.
  - `all` (default): app goes to every main category it lists.
  - `first`: app goes to the first main category in its `Categories=` order.
  - No main category: "Other".
  - Unknown `mode`: warning, treated as `all`.

`config.go`:
- `MenuItem.Mode` (`yaml:"mode"`), `MenuItem.Categories` (`yaml:"-"`, set only
  by `scanApplications`).
- `resolveInclude(item MenuItem)` gains the `applications:categories` alias;
  `mode` is ignored on other aliases.

## Configuration

```yaml
- name: Apps
  children:
    - include: applications:categories
      mode: all        # all (default) | first
```

## Testing

### Human Testing

- Add the include above, start the sidecar, open `gtk3-menus/*.xml`: one
  submenu per non-empty category, "Other" last, apps alphabetical.
- Default (no `mode`): an app listing several main categories appears in each.
- `mode: first`: each app appears once, under its first main category.
- `mode: bogus`: warning logged, output identical to `all`.

### Unit Testing

- `parseDesktopEntry`: `Categories=Network;WebBrowser;GTK;` →
  `[Network WebBrowser GTK]`; missing key → none.
- `categorizeApps`: `all` (default) places a multi-category app in each
  category; `first` places it once; non-main or missing categories → "Other" (last); empty categories
  absent; labels and ordering as above.
- `expandIncludes`: `applications:categories` yields category submenus;
  invalid `mode` equals `all`.

### Integration Testing

- `loadConfig()` with `applications:categories` and a temp `XDG_DATA_HOME`
  of sample `.desktop` files: `cfg.Menus` holds the expected submenus.

## Status

Implemented.
