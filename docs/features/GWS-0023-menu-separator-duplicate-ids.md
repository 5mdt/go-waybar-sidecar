# GWS-0023. Menu separator items and duplicate element IDs

**Tags:** #menu #config-generation

## Description

Two generator defects cause waybar to crash or fail to open a menu:

1. **Duplicate item names.** When two `MenuItem` entries in a `menu_definition`
   share the same `name`, `addNode` emits duplicate `id` attributes in the GTK
   Builder XML. `GtkBuilder` requires unique IDs; duplicates produce a segfault.

2. **Separator items.** An item with `name: "---"` is intended as a visual
   separator. Emitting it as `GtkMenuItem` with `id="---"` produces an invalid
   GObject ID; GTK rejects the file and the menu never opens.

## Implementation

`generator.go` `addNode`:
- Accepts an additional `ids map[string]int` argument tracking per-name usage
  counts, shared across the whole document.
- For `item.Name == "---"`: emits `<object class="GtkSeparatorMenuItem"
  id="sepN"/>` (where N is a counter); no entry is added to `actions`.
- For all other items: increments `ids[item.Name]`; uses `item.Name` as the
  element ID for the first occurrence and `item.Name_N` for subsequent ones
  (N ≥ 2). The actions map key uses the same unique ID.

`buildXML` initialises `ids := map[string]int{}` per menu and passes it into
every top-level `addNode` call.

## Configuration

No config change. `name: "---"` in `menu_definition` is the separator trigger.

## Testing

### Human Testing

- Add two items with the same name in `menu_definition`, regenerate, and confirm
  waybar opens the menu without crashing.
- Add a `name: "---"` item inside a submenu, regenerate, and confirm a visual
  separator appears in the popup.

### Unit Testing

- `addNode` with `name: "---"`: assert `GtkSeparatorMenuItem` is emitted and
  `actions` is not updated.
- `addNode` with two leaf items sharing the same name: assert the second item's
  element `id` is `Name_2` and both entries appear in `actions`.
- `addNode` with three items of the same name: assert IDs are `Name`, `Name_2`,
  `Name_3`.

### Integration Testing

- `buildXML` with a menu containing a separator and a duplicate name: assert the
  resulting XML file parses without error and contains `GtkSeparatorMenuItem`.

## Status

Implemented.
