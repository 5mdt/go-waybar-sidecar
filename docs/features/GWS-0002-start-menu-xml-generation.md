# GWS-0002. Start menu XML generation (start.xml)

## Description

The sidecar renders `~/.config/waybar/start.xml`, a GTK `GtkMenu` definition
used by Waybar's `custom/Start` module, from the `menu_definition` list under
`waybar.custom/Start` in `config.yaml`.

## Implementation

`generator.go`:
- `buildXML(cfg)`:
  - Creates an `etree` document with an `<interface>` root containing a
    `GtkMenu` object with `id="menu"`.
  - For each top-level entry in `cfg.StartMenu`, calls `addNode()` to append a
    `GtkMenuItem` child.
  - Indents the document (2 spaces) and writes it to `outXML`.
  - Returns the `menuActions` map built by `addNode()` (item name → action),
    consumed by `writeJSON()` (GWS-0001) for the `menu-actions` field.
- `cfg.StartMenu` is parsed in `config.go` `loadConfig()` from
  `waybar.custom/Start.menu_definition` into `[]MenuItem{Name, Action,
  Children}`.

## Configuration

- Source: `waybar.custom/Start.menu_definition` in
  `~/.config/waybar/config.yaml`. Each item has `name`, and either `action`
  (leaf) or `children` (submenu, see GWS-0003).
- Output: `~/.config/waybar/start.xml`.

## Testing

### Human Testing

- Edit `menu_definition` in `config.yaml` to add/remove a top-level item,
  trigger a regeneration, and confirm `start.xml` contains a matching
  `GtkMenuItem`.
- Click the Start module in Waybar and confirm the new item appears in the
  popup menu.

### Unit Testing

- `buildXML()` with a single leaf item `{name: "Terminal", action: "foot"}`:
  assert the output XML contains a `GtkMenuItem` with label `Terminal` and
  `menuActions["Terminal"] == "foot"`.
- `buildXML()` with an empty `cfg.StartMenu`: assert the output XML contains
  an empty `GtkMenu` and an empty `menuActions` map.

### Integration Testing

- Run `generate()` against a temp config with a representative
  `menu_definition` and assert `outXML` is well-formed XML parseable back into
  the expected item/label structure.

## Status

Implemented.
