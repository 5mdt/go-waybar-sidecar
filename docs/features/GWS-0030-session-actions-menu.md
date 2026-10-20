# GWS-0030. Session actions menu

**Tags:** #menu #config-generation #power

## Description

A menu item can pull in a list of session/power actions (lock, log out,
suspend, hibernate, hybrid sleep, suspend-then-hibernate, reboot, reboot to
firmware, shutdown) instead of listing them by hand. Only actions the system
reports as available are shown. Availability is queried on every config load.

## Implementation

`sessionactions.go`:
- `sessionActions` table, in menu order: `{name, action, method}`. `method`
  is the `org.freedesktop.login1.Manager` availability call; empty means
  always shown.

  | Name | Action | Method |
  |---|---|---|
  | Lock | `loginctl lock-session` | - |
  | Log Out | `loginctl terminate-session "$XDG_SESSION_ID"` | - |
  | Suspend | `systemctl suspend` | `CanSuspend` |
  | Hibernate | `systemctl hibernate` | `CanHibernate` |
  | Hybrid Sleep | `systemctl hybrid-sleep` | `CanHybridSleep` |
  | Suspend then Hibernate | `systemctl suspend-then-hibernate` | `CanSuspendThenHibernate` |
  | Reboot | `systemctl reboot` | `CanReboot` |
  | Reboot to Firmware | `systemctl reboot --firmware-setup` | `CanRebootToFirmwareSetup` |
  | Shutdown | `systemctl poweroff` | `CanPowerOff` |

- `scanSessionActions() []MenuItem` (package-level `var`, indirection for
  tests): calls `buildSessionActionItems` with a `can` func querying
  `org.freedesktop.login1` (`/org/freedesktop/login1`) on the system bus.
  Returns `nil` (item dropped, see `resolveInclude`) if the bus is
  unavailable.
- `buildSessionActionItems(can func(method string) string) []MenuItem`: pure;
  keeps entries with empty `method` or `can(method)` of `yes`/`challenge`.

`config.go`:
- `resolveInclude` gains a `"session-actions:plain"` case.

## Configuration

```yaml
custom/Power:
  menu_definition:
    - include: session-actions:plain
```

Rules:
- Same splicing semantics as `applications:plain` (GWS-0028).
- Switch user is not supported (see `docs/todo.md`).

## Testing

### Human Testing

- Add `- include: session-actions:plain`, start the sidecar: the generated
  menu lists only actions logind reports as `yes`/`challenge`.
- Click Lock: the session locks.

### Unit Testing

- `buildSessionActionItems`: all `yes` -> every action in table order;
  `challenge` kept; `no`/`na`/empty dropped; Lock and Log Out always present;
  `Action` matches the table.
- `expandIncludes`: `session-actions:plain` resolves via a stubbed
  `scanSessionActions`.

### Integration Testing

None; needs a live system bus.

## Status

Implemented.
