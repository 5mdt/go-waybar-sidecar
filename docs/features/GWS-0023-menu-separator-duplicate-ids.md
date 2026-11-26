# GWS-0023. Menu separator items and duplicate element IDs

**Tags:** #menu #config-generation

## Description

Menu items get unique GtkBuilder ids whatever their names are, and an item named `---` becomes a visual separator. Both are needed because duplicate ids crash waybar and `---` is not a valid GObject id. A generated document is validated for duplicate ids before it is written.

## Implementation

`src/generator.go` `addNode`:
- Takes a per-menu counter `n *int`, shared across the whole document (submenus included) and incremented once per item or separator.
- For `item.Name == "---"`: emits `<object class="GtkSeparatorMenuItem" id="sepN"/>`; no entry is added to `actions`.
- For all other items: the element ID is `itemN`, never derived from the name, so ids cannot collide whatever the names are. The actions map key is the same ID. Sub-objects use `<id>_box`, `<id>_icon`, `<id>_label`.

`buildXML` starts `n := 0` per menu and passes `&n` into every top-level `addNode` call.

`validateMenuDoc(doc *etree.Document) error` returns an error naming the first duplicate `<object>` id. `buildXML` calls it before `doc.WriteToFile`; on error it logs via `logger.Error` (notification per GWS-0013, with `notifyOpen("Open menu", path)` per GWS-0026), skips the file and omits the module from the results, so it gets no `menu-file`/`menu-actions`. Counter ids make this unreachable today; it is a guard only.

## Configuration

No config change. `name: "---"` in `menu_definition` is the separator trigger.

## Testing

### Human Testing

- Add two items with the same name in `menu_definition`, regenerate, and confirm waybar opens the menu without crashing.
- Add a `name: "---"` item inside a submenu, regenerate, and confirm a visual separator appears in the popup.

### Unit Testing

- `addNode` with `name: "---"`: assert `GtkSeparatorMenuItem` is emitted and `actions` is not updated.
- `buildXML` with three items of the same name: assert IDs are `item1`, `item2`, `item3` and each appears in `actions`.
- Names `Foo`, `Foo`, `Foo_2`, `sep1`, `---` together: assert all ids are unique and `validateMenuDoc` passes.
- `validateMenuDoc` with two `<object id="X">`: non-nil error; unique ids: `nil`.

### Integration Testing

- `buildXML` with a menu containing a separator and a duplicate name: assert the resulting XML file parses without error and contains `GtkSeparatorMenuItem`.

## Status

Implemented.
