# GWS-0029. Power profile menu

**Tags:** #menu #config-generation #power

## Description

A menu item can pull in a list of available `power-profiles-daemon` profiles (power-saver / balanced / performance) instead of listing them by hand. The currently active profile is rendered bold with a leading `●`, the others indented to align. Clicking an entry switches the active profile via `powerprofilesctl`. The active-profile marker is kept live: the sidecar subscribes to the daemon's D-Bus `PropertiesChanged` signal and regenerates the menu immediately when the profile changes, including changes made outside the sidecar (GNOME Settings, another `powerprofilesctl` call, an auto-switch on AC/battery).

## Implementation

`powerprofiles.go`:
- Talks to `net.hadess.PowerProfiles` on the system bus (`/net/hadess/PowerProfiles`), the same `github.com/godbus/dbus/v5` already used by `notify.go`.
- `scanPowerProfiles() []MenuItem` (package-level `var`, indirection for tests): reads the `Profiles` (`aa{sv}`) and `ActiveProfile` (`s`) properties and calls `buildPowerProfileItems`. Returns `nil` (menu item dropped, see `resolveInclude`) if the bus or the daemon is unavailable - best-effort, matches `newNotifier`'s pattern.
- `parseProfileIDs(v dbus.Variant) []string`: extracts profile ids from the `Profiles` property in daemon-returned order (not re-sorted - this order is already low-to-high, e.g. power-saver, balanced, performance).
- `powerProfileDisplayName(id string) string`: hyphenated id to Title Case, e.g. `power-saver` -> `Power Saver`.
- `buildPowerProfileItems(ids []string, active string) []MenuItem`: one `MenuItem` per id, `Action: "powerprofilesctl set " + id`. The active id gets `Bold: true` and a `"● "` name prefix; others get a `" "` prefix so labels stay left-aligned regardless of which one is bold.
- `watchPowerProfiles(trigger func())` (package-level `var`): subscribes to `org.freedesktop.DBus.Properties.PropertiesChanged` on the power-profiles object path and calls `trigger` whenever `ActiveProfile` is among the changed/invalidated properties. No-ops (logs at debug) if the system bus or the daemon isn't available.
- `isActiveProfileChange(name string, body []interface{}) bool`: pure decode-and-match helper, split out so it's testable without a bus connection.

`config.go`:
- `resolveInclude` gains a `"power-profiles:plain"` case, resolving via `scanPowerProfiles()`.

`generator.go`:
- `MenuItem.Bold` (`yaml:"-"`, set only by resolvers) controls label rendering in `setMenuItemLabel()`. Non-bold items use the `label` property. Bold items get a child `GtkLabel` with `use-markup=True` and `<b>...</b>`, because `GtkMenuItem.label` does not interpret markup.

`watcher.go`:
- The debounced reload trigger (GWS-0005) is shared between fsnotify events and a `go watchPowerProfiles(trigger)` goroutine started by `run()`, so an external profile change reloads the same way an edited `config.yaml` does (GWS-0005).

## Configuration

```yaml
custom/Power:
  menu_definition:
    - name: Power Profile
      children:
        - include: power-profiles:plain
    - name: "---"
    - name: Shutdown
      action: "systemctl poweroff"
```

Rules:
- Same splicing semantics as `applications:plain` (GWS-0028): `include: power-profiles:plain` expands in place, at top level or inside `children`.
- Requires `power-profiles-daemon` running and `powerprofilesctl` on `PATH` for the click action; if the daemon isn't reachable at generate time the item is dropped with a warning and the rest of the menu still generates.

## Testing

### Human Testing

- Add `- include: power-profiles:plain` under a menu item, start the sidecar, open the generated menu: one entry per profile, the active one bold with a leading `●`.
- Click a non-active profile: `powerprofilesctl get` reflects the switch, and the sidecar log shows a reload triggered by the D-Bus signal - reopening the menu shows the new profile bold.
- Run `powerprofilesctl set <other>` from a separate terminal while the sidecar is running: reopening the menu shows the change without editing `config.yaml`.
- Stop `power-profiles-daemon` (or run on a machine without it) and start the sidecar: it logs a warning and starts normally; a menu using `include: power-profiles:plain` is generated without that item.

### Unit Testing

- `powerProfileDisplayName`: `power-saver` -> `Power Saver`, `balanced` -> `Balanced`, single-word and multi-hyphen ids.
- `parseProfileIDs`: a constructed `dbus.Variant` wrapping the `aa{sv}` shape yields ids in order; malformed/empty variant yields `nil`.
- `buildPowerProfileItems`: active id gets `Bold: true` and the `●` prefix; others get the plain-indent prefix; `Action` is `powerprofilesctl set <id>` for every item; order matches input order.
- `isActiveProfileChange`: `PropertiesChanged` body naming `net.hadess.PowerProfiles` with `ActiveProfile` in the changed map -> `true`; same interface with only an unrelated property -> `false`; `ActiveProfile` in the invalidated-properties list -> `true`; a different interface name -> `false`.
- `setMenuItemLabel`: bold renders a child `GtkLabel` with `use-markup=True` and `<b>label</b>` text (label escaped with `html.EscapeString`), no `label` property on the `GtkMenuItem` itself; non-bold renders the plain `label` property.
- `resolveInclude("power-profiles:plain")`: resolves via a stubbed `scanPowerProfiles`.

### Integration Testing

- `buildXML` over a `Config` with a bold `MenuItem`: the written XML file contains `use-markup` and the bold-wrapped label text, and `validateMenuDoc` still passes (label's child id doesn't collide with the item's own id).

## Status

Implemented.
