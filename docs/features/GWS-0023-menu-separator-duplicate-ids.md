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
- Takes a per-menu counter `n *int`, shared across the whole document
  (submenus included) and incremented once per item or separator.
- For `item.Name == "---"`: emits `<object class="GtkSeparatorMenuItem"
  id="sepN"/>`; no entry is added to `actions`.
- For all other items: the element ID is `itemN`, never derived from the
  name, so ids cannot collide whatever the names are. The actions map key is
  the same ID. Sub-objects use `<id>_box`, `<id>_icon`, `<id>_label`.

`buildXML` starts `n := 0` per menu and passes `&n` into every top-level
`addNode` call.

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
- `buildXML` with three items of the same name: assert IDs are `item1`,
  `item2`, `item3` and each appears in `actions`.
- Names `Foo`, `Foo`, `Foo_2`, `sep1`, `---` together: assert all ids are
  unique and `validateMenuDoc` passes.

### Integration Testing

- `buildXML` with a menu containing a separator and a duplicate name: assert the
  resulting XML file parses without error and contains `GtkSeparatorMenuItem`.

## Status

Implemented.
