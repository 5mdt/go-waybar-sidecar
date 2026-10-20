package main

import "testing"

func TestBuildSessionActionItems_AllAvailable(t *testing.T) {
	items := buildSessionActionItems(func(string) string { return "yes" })

	if len(items) != len(sessionActions) {
		t.Fatalf("got %d items, want %d", len(items), len(sessionActions))
	}
	for i, a := range sessionActions {
		if items[i].Name != a.name || items[i].Action != a.action {
			t.Errorf("items[%d] = %+v, want %q / %q", i, items[i], a.name, a.action)
		}
	}
}

func TestBuildSessionActionItems_Availability(t *testing.T) {
	answers := map[string]string{
		"CanSuspend":   "challenge",
		"CanHibernate": "no",
		"CanReboot":    "na",
	}
	items := buildSessionActionItems(func(m string) string { return answers[m] })

	got := map[string]bool{}
	for _, it := range items {
		got[it.Name] = true
	}
	for _, name := range []string{"Lock", "Log Out", "Suspend"} {
		if !got[name] {
			t.Errorf("expected %q to be present", name)
		}
	}
	for _, name := range []string{"Hibernate", "Reboot", "Shutdown", "Hybrid Sleep"} {
		if got[name] {
			t.Errorf("expected %q to be dropped", name)
		}
	}
}
