package main

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestPowerProfileDisplayName(t *testing.T) {
	cases := map[string]string{
		"power-saver": "Power Saver",
		"balanced":    "Balanced",
		"performance": "Performance",
		"a-b-c":       "A B C",
	}
	for id, want := range cases {
		if got := powerProfileDisplayName(id); got != want {
			t.Errorf("powerProfileDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestParseProfileIDs(t *testing.T) {
	v := dbus.MakeVariant([]map[string]dbus.Variant{
		{"Profile": dbus.MakeVariant("power-saver"), "Driver": dbus.MakeVariant("platform_profile")},
		{"Profile": dbus.MakeVariant("balanced"), "Driver": dbus.MakeVariant("platform_profile")},
		{"Profile": dbus.MakeVariant("performance"), "Driver": dbus.MakeVariant("platform_profile")},
	})

	got := parseProfileIDs(v)
	want := []string{"power-saver", "balanced", "performance"}
	if len(got) != len(want) {
		t.Fatalf("parseProfileIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseProfileIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseProfileIDs_Malformed(t *testing.T) {
	if got := parseProfileIDs(dbus.MakeVariant("not a profile list")); got != nil {
		t.Errorf("parseProfileIDs() = %v, want nil", got)
	}
}

func TestBuildPowerProfileItems(t *testing.T) {
	items := buildPowerProfileItems([]string{"power-saver", "balanced", "performance"}, "balanced")

	if len(items) != 3 {
		t.Fatalf("buildPowerProfileItems() = %d items, want 3", len(items))
	}

	for i, id := range []string{"power-saver", "balanced", "performance"} {
		wantAction := "powerprofilesctl set " + id
		if items[i].Action != wantAction {
			t.Errorf("items[%d].Action = %q, want %q", i, items[i].Action, wantAction)
		}
	}

	if !items[1].Bold {
		t.Error("expected active profile (balanced) to be Bold")
	}
	if items[0].Bold || items[2].Bold {
		t.Error("expected only the active profile to be Bold")
	}

	if items[1].Name != "● Balanced" {
		t.Errorf("active Name = %q, want %q", items[1].Name, "● Balanced")
	}
	if items[0].Name != "  Power Saver" {
		t.Errorf("inactive Name = %q, want %q", items[0].Name, "  Power Saver")
	}
}

func TestIsActiveProfileChange_ChangedProperty(t *testing.T) {
	body := []interface{}{
		"net.hadess.PowerProfiles",
		map[string]dbus.Variant{"ActiveProfile": dbus.MakeVariant("performance")},
		[]string{},
	}
	if !isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", body) {
		t.Error("expected true for ActiveProfile in changed properties")
	}
}

func TestIsActiveProfileChange_InvalidatedProperty(t *testing.T) {
	body := []interface{}{
		"net.hadess.PowerProfiles",
		map[string]dbus.Variant{},
		[]string{"ActiveProfile"},
	}
	if !isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", body) {
		t.Error("expected true for ActiveProfile in invalidated properties")
	}
}

func TestIsActiveProfileChange_UnrelatedProperty(t *testing.T) {
	body := []interface{}{
		"net.hadess.PowerProfiles",
		map[string]dbus.Variant{"PerformanceDegraded": dbus.MakeVariant("")},
		[]string{},
	}
	if isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", body) {
		t.Error("expected false when ActiveProfile is not among changed/invalidated properties")
	}
}

func TestIsActiveProfileChange_WrongInterface(t *testing.T) {
	body := []interface{}{
		"org.some.OtherInterface",
		map[string]dbus.Variant{"ActiveProfile": dbus.MakeVariant("performance")},
		[]string{},
	}
	if isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", body) {
		t.Error("expected false for a PropertiesChanged signal on an unrelated interface")
	}
}

func TestIsActiveProfileChange_MalformedBody(t *testing.T) {
	if isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", nil) {
		t.Error("expected false for empty body")
	}
	if isActiveProfileChange("org.freedesktop.DBus.Properties.PropertiesChanged", []interface{}{1, 2, 3}) {
		t.Error("expected false for wrongly-typed body")
	}
}
