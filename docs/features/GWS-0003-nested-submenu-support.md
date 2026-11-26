# GWS-0003. Nested submenu support

**Tags:** #menu

## Description

Items in `menu_definition` may contain `children`, which are rendered as a nested `GtkMenu` submenu instead of a clickable action.

## Implementation

`generator.go` `addNode(parent, item, actions, n, withIcon)`:
- Always creates a `GtkMenuItem` with a unique `id` (`item<N>`, see GWS-0023).
- If `item.Children` is empty, the item is a leaf: its label is `item.Name` and `actions[id] = item.Action` is recorded for `menu-actions` (GWS-0001).
- If `item.Children` is non-empty, the item is a submenu:
  - Its label is `item.Name`, unchanged from a leaf item.
  - A `<child type="submenu">` containing a new `GtkMenu` object is created.
  - `addNode()` is called recursively for each entry in `item.Children`, appending into the new submenu. Submenu items are not added to `actions` - only leaves contribute actions.
- Recursion has no depth limit; nesting depth is whatever `menu_definition` declares.

## Configuration

- Source: `children` field of any entry in `waybar.<module>.menu_definition` (`config.yaml`). A `children` entry follows the same `MenuItem` shape (`name`, `action`, `children`), so submenus can themselves contain submenus.

## Testing

### Human Testing

- Add a `children` list to a `menu_definition` entry (e.g. the default `Apps`/`Power` groups in `config.yaml`), regenerate, and confirm the Start menu shows a submenu that opens to reveal the children.
- Nest a submenu inside a submenu and confirm both levels render correctly.

### Unit Testing

- `addNode()` with an item that has `children`: assert the produced `GtkMenuItem` contains a `child[type=submenu]` > `GtkMenu`, and that the item itself is not added to `actions`.
- `addNode()` with a leaf item: assert no submenu element is created and `actions[id] == item.Action`.
- `addNode()` with two levels of nesting: assert both levels produce nested `GtkMenu` elements and only leaf items appear in `actions`.

### Integration Testing

- Run `generate()` against a config with a multi-level `menu_definition` and assert the menu XML parses into the expected nested structure and `config.jsonc`'s `menu-actions` contains only the leaf actions.

## Status

Implemented.
