package main

import "github.com/godbus/dbus/v5"

const (
	login1BusName    = "org.freedesktop.login1"
	login1ObjectPath = "/org/freedesktop/login1"
	login1Manager    = "org.freedesktop.login1.Manager"
)

// sessionActions lists the session/power menu entries in menu order. method
// is the logind availability call; empty means always shown.
var sessionActions = []struct{ name, action, method string }{
	{"Lock", "loginctl lock-session", ""},
	{"Log Out", `loginctl terminate-session "$XDG_SESSION_ID"`, ""},
	{"Suspend", "systemctl suspend", "CanSuspend"},
	{"Hibernate", "systemctl hibernate", "CanHibernate"},
	{"Hybrid Sleep", "systemctl hybrid-sleep", "CanHybridSleep"},
	{"Suspend then Hibernate", "systemctl suspend-then-hibernate", "CanSuspendThenHibernate"},
	{"Reboot", "systemctl reboot", "CanReboot"},
	{"Reboot to Firmware", "systemctl reboot --firmware-setup", "CanRebootToFirmwareSetup"},
	{"Shutdown", "systemctl poweroff", "CanPowerOff"},
}

// scanSessionActions returns the session actions logind reports as available,
// or nil if the system bus is unavailable. Package-level var so tests can
// stub it.
var scanSessionActions = func() []MenuItem {
	conn, err := dbus.SystemBus()
	if err != nil {
		logger.Debug("session actions unavailable", "err", err)
		return nil
	}
	obj := conn.Object(login1BusName, dbus.ObjectPath(login1ObjectPath))

	return buildSessionActionItems(func(method string) string {
		var res string
		if err := obj.Call(login1Manager+"."+method, 0).Store(&res); err != nil {
			logger.Debug("failed to query logind", "method", method, "err", err)
			return ""
		}
		return res
	})
}

// buildSessionActionItems keeps actions with no availability method, or whose
// method reports "yes" or "challenge".
func buildSessionActionItems(can func(method string) string) []MenuItem {
	items := make([]MenuItem, 0, len(sessionActions))
	for _, a := range sessionActions {
		if a.method != "" {
			if r := can(a.method); r != "yes" && r != "challenge" {
				continue
			}
		}
		items = append(items, MenuItem{Name: a.name, Action: a.action})
	}
	return items
}
