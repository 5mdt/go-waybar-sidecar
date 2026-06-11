# GWS-0015. Multiple menu support across waybar modules

## Description

Any module under `waybar:` in `config.yaml` that has a `menu_definition`
field gets its own GTK `GtkMenu` XML file, `menu: "on-click"`, `menu-file`,
and `menu-actions` wired up automatically — not just `custom/Start`. This
lets users define independent popup menus for multiple modules (e.g.
`custom/Start` and `custom/Power`) in the same config.

## Implementation

`config.go` `loadConfig()`:
- After the first unmarshal pass into `raw.Waybar` (`map[string]any`),
  iterates every top-level key in `raw.Waybar`. For each value that is a
  `map[string]any` and contains a `menu_definition` key, the value is
  re-marshaled to YAML and unmarshaled into `[]MenuItem` (round-trip via
  `yaml.Marshal`/`yaml.Unmarshal`, reusing the existing tagged struct —
  no second typed pass needed).
- Result is `Config.Menus map[string][]MenuItem`, keyed by module name
  (e.g. `"custom/Start"`, `"custom/Power"`). Modules without
  `menu_definition` (e.g. `clock`, `layer`, `modules-left`) are skipped.

`generator.go`:
- `buildXML(cfg)` iterates `cfg.Menus` (sorted by module name for
  deterministic output), building one `GtkMenu` document per menu via the
  existing `addNode()` (GWS-0003 nested submenus apply per-menu as before).
  Each document is written to `gtk3-menus/<menuFileName(module)>.xml`.
- `menuFileName(module)` derives the filename: lowercase the module name and
  replace `/` with `-`, e.g. `custom/Start` → `custom-start.xml`,
  `custom/Power` → `custom-power.xml`.
- `writeJSON(waybar, menus)` iterates the same `menus` map: for each module
  name present in `waybar`, removes `menu_definition` and sets `menu:
  "on-click"`, `menu-file: <path>`, `menu-actions: <map>`. Modules without an
  entry in `menus` (no `menu_definition`) are left untouched.

## Configuration

- Any `waybar.<module>.menu_definition` list (same `name`/`action`/`children`
  shape as before, see GWS-0002/GWS-0003) opts that module into menu
  generation. No new top-level config keys.

## Testing

### Human Testing

- Add a second module with `menu_definition` (e.g. `custom/Power`) to
  `config.yaml`, alongside the existing `custom/Start`. Trigger a
  regeneration and confirm:
  - `~/.config/waybar/gtk3-menus/custom-start.xml` and
    `~/.config/waybar/gtk3-menus/custom-power.xml` both exist.
  - `config.jsonc` has `menu`/`menu-file`/`menu-actions` set for both modules
    and no `menu_definition` field on either.
- Click each module in Waybar and confirm its own popup menu appears with the
  correct items.

### Unit Testing

- `loadConfig()` against a config with two modules each having
  `menu_definition`: assert `cfg.Menus` has both entries with the correct
  items (including nested `children`), and modules without
  `menu_definition` are absent from `cfg.Menus`.
- `menuFileName()`: `"custom/Start"` → `"custom-start.xml"`, `"custom/Power"`
  → `"custom-power.xml"`, `"clock"` → `"clock.xml"`.
- `buildXML()` with `cfg.Menus` containing two modules: assert two XML files
  are written under `menuDir`, one per module.
- `writeJSON()` with `menus` containing two modules: assert both
  corresponding entries in the output JSON get `menu`/`menu-file`/
  `menu-actions` and have `menu_definition` removed; a module absent from
  `menus` is unchanged.

### Integration Testing

- Run `generate()` against a temp config with two menu-bearing modules and
  assert both XML files exist and `config.jsonc` references each by its own
  `menu-file`.

## Status

Implemented.
