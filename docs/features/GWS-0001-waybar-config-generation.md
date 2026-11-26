# GWS-0001. Waybar config generation (config.jsonc)

**Tags:** #config-generation

## Description

The sidecar renders `~/.config/waybar/config.jsonc` from the `waybar` section
of `~/.config/waybar/config.yaml`, transforming it into the menu-driven config
that Waybar consumes.

## Implementation

`generator.go`:
- `generate()` calls `buildXML()` first (see GWS-0002/GWS-0015) to obtain a
  `map[string]menuResult` (one entry per menu-bearing module), then
  `writeJSON(cfg.Waybar, menus)`.
- `jsonSafe()` returns a deep copy of the tree with `map[interface{}]interface{}`
  values (non-string YAML keys) converted to `map[string]any`, so it can be
  marshaled by `encoding/json`. The input is never modified.
- `writeJSON()`:
  - Copies `cfg.Waybar` via `jsonSafe()`; `cfg` is never modified.
  - For each module name present in `menus` (see GWS-0015), removes its
    `menu_definition` field and sets `menu: "on-click"`, `menu-file:
    <path>`, and `menu-actions: <map>`.
  - Encodes the result as indented JSON to `outJSON`, preceded by a header of `//` comment lines: "This file is generated, do not
    edit manually", then `generation date` (RFC3339), `generation app name`,
    `generation app version` (`main.version`, set by `-ldflags -X
    main.version=...`, default `dev`) and `generation source` (`configPath`).
    Waybar's JSONC parser accepts comments.

## Configuration

- Source: `waybar` key of `~/.config/waybar/config.yaml`.
- Output: `~/.config/waybar/config.jsonc`.

## Testing

### Human Testing

- Edit `~/.config/waybar/config.yaml` (e.g. add a module under `waybar`),
  trigger a regeneration, and confirm the new module appears in
  `config.jsonc`.
- Confirm `custom/Start` in `config.jsonc` has `menu`, `menu-file`, and
  `menu-actions` set and no `menu_definition` field.

### Unit Testing

- `jsonSafe()` with nested `map[interface{}]interface{}` and `[]any` values:
  assert the result is composed entirely of `map[string]any`, and the input
  is unchanged.
- `writeJSON()`: assert `cfg.Waybar` still has `menu_definition` and no
  `menu-file` afterwards.
- `writeJSON()` with a `menus` map containing an entry for `custom/Start`:
  assert the written JSON has `menu_definition` removed and
  `menu`/`menu-file`/`menu-actions` set for that module.
- `writeJSON()`: assert `outJSON` starts with the header comment lines
  (contains app name, version and config path) and the rest is valid JSON.
- `writeJSON()` with a `menus` map that has no entry for a given module:
  assert that module is written unchanged (aside from `jsonSafe()`). See
  GWS-0015 for the multi-module case.

### Integration Testing

- Run `generate()` against a temp config with a representative `waybar`
  section and assert `outJSON` is valid JSON matching the expected structure.

## Status

Implemented.
