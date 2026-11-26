# GWS-0032. Menu item icons

**Tags:** #menu #config-generation

## Description

A menu item can show a 16px icon left of its label. Icons come from the `icon:` field of a `config.yaml` item and from `Icon=` of `.desktop` entries in the generated application list (GWS-0028); category submenus (GWS-0031) get fixed icons.

## Implementation

- `MenuItem.Icon` is set from `icon:` (config) or `Icon=` (`scanApplications`).
- `icon` is a glyph when it is a single character, a file path when it contains `/`, otherwise an icon-theme name.
- `buildXML`/`addNode` use the icon layout for every non-separator item of a menu level when at least one item of that level has an `Icon`; levels without icons are unchanged.
- Icon layout: the `GtkMenuItem` child is a horizontal `GtkBox` holding a `GtkImage` (`pixel-size` 16) and the `GtkLabel` (bold markup as in GWS-0029).
- The image sets `icon-name` (theme name) or `file` (path) only when the icon is usable: non-empty and, for a path, existing. Otherwise the image stays blank, so labels of one level stay aligned. `gicon` is avoided: GtkBuilder reads it as an object id and fails the whole menu.
- `pixel-size` does not scale `file` images; path icons should be 16px.
- A glyph slot is a `GtkLabel` (`width-request` 16, `xalign` 0.5) in place of the `GtkImage`; it uses the menu font, so a Nerd Font (or fallback) is needed.
- Theme names are not validated; GTK shows its broken-image icon for unknown names.

## Configuration

```yaml
- name: Shutdown
  icon: system-shutdown
  action: "systemctl poweroff"
- name: Notes
  icon: /home/me/icons/notes.png
  action: "notes"
- name: Launcher
  icon: "\U000F0463"   # glyph
  action: "launcher"
```

## Testing

### Human Testing

- Add `icon:` to config items and regenerate: icons appear, items without an icon stay aligned.
- Open the application list: apps show their `.desktop` icons.

### Unit Testing

- `parseDesktopEntry` reads `Icon=`.
- `addNode` with a theme icon emits `GtkBox`, `GtkImage` with `icon-name`, and `GtkLabel`.
- A one-character icon emits a `GtkLabel` slot with that glyph, no `GtkImage`.
- A sibling without an icon gets a `GtkImage` without icon property.
- A non-existing icon path gets no `file` property.
- A bold item with an icon keeps its markup label.
- A level without icons emits no `GtkBox`/`GtkImage`.

### Integration Testing

- `buildXML` with mixed icon/no-icon items: ids stay unique (`validateMenuDoc` passes).

## Status

Implemented.
