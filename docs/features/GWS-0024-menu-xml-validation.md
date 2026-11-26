# GWS-0024. Menu XML duplicate ID validation

**Tags:** #menu #config-generation

## Description

After building a GtkMenu XML document in memory, the generator validates it
before writing to disk. Any future regression that produces duplicate GtkBuilder
`id` attributes is caught at generation time and reported rather than silently
crashing waybar.

## Implementation

`generator.go`:

- `validateMenuDoc(doc *etree.Document) error`: iterates every `<object>` element
  with an `id` attribute; returns an error naming the first duplicate id found,
  or `nil` if all ids are unique.
- `buildXML` calls `validateMenuDoc(doc)` after building each document and
  before `doc.WriteToFile`. On error: logs via `logger.Error` (which triggers a
  desktop notification per GWS-0013), skips writing the file, and omits the menu
  from `results`.
- Guard only: counter ids (GWS-0023) make duplicates impossible by
  construction, so the skip path is not reachable from `buildXML` today.

**Consequence of skip:** the skipped module gets no `menu-file` / `menu-actions`
entry in the regenerated `config.jsonc`. Its previous XML file on disk is left
untouched. All valid menus are unaffected.

## Configuration

None. Validation is unconditional.

## Testing

### Human Testing

- Normal operation: regenerate against `config.yaml`; confirm the menu XML file
  is written and waybar opens the menu.

### Unit Testing

- `TestValidateMenuDocDetectsDuplicateIDs`: build an etree doc with two
  `<object id="X">` and assert a non-nil error; a doc with unique ids returns
  `nil`.

### Integration Testing

- `buildXML` with a well-formed config produces all expected XML files and all
  ids in each file are unique (covered by existing `TestBuildXMLAndWriteJSON`).

## Status

Implemented.
