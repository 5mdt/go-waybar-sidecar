# GWS-0002. Menu XML generation (GtkMenu documents)

**Tags:** #config-generation #menu

## Description

The sidecar renders one GTK `GtkMenu` XML document per menu-bearing module
(see GWS-0015), e.g. `~/.config/waybar/gtk3-menus/custom-start.xml` for
`waybar.custom/Start`, from that module's `menu_definition` list in
`config.yaml`.

## Implementation

`generator.go`:
- `buildXML(cfg)`, for each entry in `cfg.Menus`:
  - Creates an `etree` document with an `<interface>` root containing a
    `GtkMenu` object with `id="menu"`.
  - For each top-level entry in the menu's items, calls `addNode()` to
    append a `GtkMenuItem` child.
  - Indents the document (2 spaces) and writes it to
    `gtk3-menus/<menuFileName(module)>.xml`.
  - Returns a `menuResult{path, actions}` per module, where `actions` is the
    `menuActions` map built by `addNode()` (item name → action), consumed by
    `writeJSON()` (GWS-0001) for the `menu-actions` field.
- `cfg.Menus["custom/Start"]` is parsed in `config.go` `loadConfig()` from
  `waybar.custom/Start.menu_definition` into `[]MenuItem{Name, Action,
  Children}` (see GWS-0015 for how modules are discovered generically).

## Configuration

- Source: `waybar.<module>.menu_definition` in
  `~/.config/waybar/config.yaml`. Each item has `name`, and either `action`
  (leaf) or `children` (submenu, see GWS-0003).
- Output: `~/.config/waybar/gtk3-menus/<menuFileName(module)>.xml`, e.g.
  `custom-start.xml` for `custom/Start`. The `gtk3-menus` directory is
  created automatically if missing.

## Testing

### Human Testing

- Edit `menu_definition` in `config.yaml` to add/remove a top-level item,
  trigger a regeneration, and confirm the corresponding XML file contains a
  matching `GtkMenuItem`.
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
  `menu_definition` and assert the resulting XML file is well-formed and
  parseable back into the expected item/label structure.

## Status

Implemented.
