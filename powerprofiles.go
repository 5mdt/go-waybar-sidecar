package main

import (
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	powerProfilesBusName    = "net.hadess.PowerProfiles"
	powerProfilesObjectPath = "/net/hadess/PowerProfiles"
	powerProfilesInterface  = "net.hadess.PowerProfiles"

	propertiesChangedSignal = "org.freedesktop.DBus.Properties.PropertiesChanged"
)

// scanPowerProfiles queries power-profiles-daemon over D-Bus for the
// available profiles and the active one, returning nil (best-effort, mirrors
// newNotifier in notify.go) if the bus or the daemon is unavailable.
// Package-level var so tests can stub it.
var scanPowerProfiles = func() []MenuItem {
	conn, err := dbus.SystemBus()
	if err != nil {
		logger.Debug("power profiles unavailable", "err", err)
		return nil
	}
	obj := conn.Object(powerProfilesBusName, dbus.ObjectPath(powerProfilesObjectPath))

	profiles, err := obj.GetProperty(powerProfilesInterface + ".Profiles")
	if err != nil {
		logger.Debug("failed to read power profiles", "err", err)
		return nil
	}
	active, err := obj.GetProperty(powerProfilesInterface + ".ActiveProfile")
	if err != nil {
		logger.Debug("failed to read active power profile", "err", err)
		return nil
	}

	activeID, _ := active.Value().(string)
	return buildPowerProfileItems(parseProfileIDs(profiles), activeID)
}

// parseProfileIDs extracts profile ids, in daemon-returned order, from the
// Profiles property (an array of {"Profile": s, "Driver": s} dicts).
func parseProfileIDs(v dbus.Variant) []string {
	entries, ok := v.Value().([]map[string]dbus.Variant)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if p, ok := e["Profile"].Value().(string); ok {
			ids = append(ids, p)
		}
	}
	return ids
}

// buildPowerProfileItems renders profiles (in daemon order) as MenuItems.
// The active profile is bold with a leading dot; others are indented to
// match, so labels stay left-aligned regardless of which one is active.
func buildPowerProfileItems(ids []string, active string) []MenuItem {
	items := make([]MenuItem, 0, len(ids))
	for _, id := range ids {
		name := powerProfileDisplayName(id)
		bold := id == active
		if bold {
			name = "● " + name
		} else {
			name = "  " + name
		}
		items = append(items, MenuItem{
			Name:   name,
			Action: "powerprofilesctl set " + id,
			Bold:   bold,
		})
	}
	return items
}

// powerProfileDisplayName title-cases a hyphenated profile id, e.g.
// "power-saver" -> "Power Saver".
func powerProfileDisplayName(id string) string {
	words := strings.Split(id, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// watchPowerProfiles subscribes to power-profiles-daemon's PropertiesChanged
// signal and calls trigger whenever ActiveProfile changes, so an external
// profile switch (GNOME Settings, another powerprofilesctl call, an
// auto-switch) regenerates the menu the same way an edited config.yaml does.
// No-ops (logs at debug) if the system bus or the daemon isn't available.
// Package-level var so tests can stub it.
var watchPowerProfiles = func(trigger func()) {
	conn, err := dbus.SystemBus()
	if err != nil {
		logger.Debug("power profiles watch disabled", "err", err)
		return
	}

	ch := make(chan *dbus.Signal, 4)
	conn.Signal(ch)
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(dbus.ObjectPath(powerProfilesObjectPath)),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		logger.Debug("power profiles watch disabled", "err", err)
		return
	}

	for sig := range ch {
		if isActiveProfileChange(sig.Name, sig.Body) {
			trigger()
		}
	}
}

// isActiveProfileChange reports whether a PropertiesChanged signal body
// names net.hadess.PowerProfiles and includes ActiveProfile among either the
// changed or invalidated properties. Split out from watchPowerProfiles so it
// is testable without a bus connection.
func isActiveProfileChange(name string, body []interface{}) bool {
	if name != propertiesChangedSignal || len(body) < 3 {
		return false
	}

	iface, ok := body[0].(string)
	if !ok || iface != powerProfilesInterface {
		return false
	}

	if changed, ok := body[1].(map[string]dbus.Variant); ok {
		if _, ok := changed["ActiveProfile"]; ok {
			return true
		}
	}

	if invalidated, ok := body[2].([]string); ok {
		for _, p := range invalidated {
			if p == "ActiveProfile" {
				return true
			}
		}
	}

	return false
}
