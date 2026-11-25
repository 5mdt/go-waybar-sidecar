package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestUrgencyHint(t *testing.T) {
	cases := []struct {
		level slog.Level
		want  byte
	}{
		{slog.LevelError, 2},
		{slog.LevelWarn, 1},
		{slog.LevelInfo, 0},
		{slog.LevelDebug, 0},
	}
	for _, c := range cases {
		if got := urgencyHint(c.level); got != c.want {
			t.Errorf("urgencyHint(%v) = %v, want %v", c.level, got, c.want)
		}
	}
}

func TestParseNotifyLevel(t *testing.T) {
	t.Run("off disables", func(t *testing.T) {
		_, enabled, err := parseNotifyLevel("off")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if enabled {
			t.Fatal("expected enabled=false for \"off\"")
		}
	})

	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}
	for _, c := range cases {
		level, enabled, err := parseNotifyLevel(c.in)
		if err != nil {
			t.Fatalf("parseNotifyLevel(%q) unexpected error: %v", c.in, err)
		}
		if !enabled {
			t.Fatalf("parseNotifyLevel(%q) expected enabled=true", c.in)
		}
		if level != c.want {
			t.Errorf("parseNotifyLevel(%q) = %v, want %v", c.in, level, c.want)
		}
	}

	t.Run("invalid value errors", func(t *testing.T) {
		if _, _, err := parseNotifyLevel("bogus"); err == nil {
			t.Fatal("expected error for invalid notify level")
		}
	})
}

func TestNotifyHandlerEnabled(t *testing.T) {
	h := &notifyHandler{threshold: slog.LevelWarn}

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info to be disabled below warn threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("expected warn to be enabled at warn threshold")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("expected error to be enabled above warn threshold")
	}
}

// newTestHandler returns a handler wired to a buffered queue the test reads.
func newTestHandler(threshold slog.Level) (*notifyHandler, chan notification) {
	q := make(chan notification, 4)
	return &notifyHandler{threshold: threshold, queue: q}, q
}

func TestNotifyHandlerHandle(t *testing.T) {
	h, q := newTestHandler(slog.LevelWarn)

	below := slog.NewRecord(time.Now(), slog.LevelInfo, "below threshold", 0)
	if err := h.Handle(context.Background(), below); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q) != 0 {
		t.Fatalf("expected nothing queued for below-threshold record, got %d", len(q))
	}

	rec := slog.NewRecord(time.Now(), slog.LevelError, "waybar failed to start", 0)
	rec.AddAttrs(slog.String("err", "boom"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q) != 1 {
		t.Fatalf("expected 1 queued notification, got %d", len(q))
	}
	got := <-q
	if got.urgency != 2 {
		t.Errorf("urgency = %v, want 2", got.urgency)
	}
	if got.summary != "waybar failed to start" {
		t.Errorf("summary = %q, want %q", got.summary, "waybar failed to start")
	}
	if !strings.Contains(got.body, "err=boom") {
		t.Errorf("body = %q, want it to contain %q", got.body, "err=boom")
	}
}

func TestNotifyHandlerHandleDefaultAction(t *testing.T) {
	h, q := newTestHandler(slog.LevelError)
	rec := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := <-q
	if got.label != "Open config" {
		t.Errorf("label = %q, want %q", got.label, "Open config")
	}
	if got.path != configPath {
		t.Errorf("path = %q, want %q", got.path, configPath)
	}
}

func TestNotifyHandlerHandleOverrideAction(t *testing.T) {
	h, q := newTestHandler(slog.LevelWarn)
	rec := slog.NewRecord(time.Now(), slog.LevelWarn, "job failed", 0)
	rec.AddAttrs(slog.String("name", "sync-mail"), notifyOpen("Show output", "/tmp/x"))

	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := <-q
	if got.label != "Show output" || got.path != "/tmp/x" {
		t.Errorf("label/path = %q/%q, want %q/%q", got.label, got.path, "Show output", "/tmp/x")
	}
	if strings.Contains(got.body, notifyActionKey) {
		t.Errorf("body = %q, should not contain %q", got.body, notifyActionKey)
	}
	if !strings.Contains(got.body, "name=sync-mail") {
		t.Errorf("body = %q, want it to contain %q", got.body, "name=sync-mail")
	}
}

func TestNotifyHandlerHandleFullQueueDoesNotBlock(t *testing.T) {
	for name, q := range map[string]chan notification{
		"full":  make(chan notification), // unbuffered, nobody reading
		"unset": nil,
	} {
		t.Run(name, func(t *testing.T) {
			h := &notifyHandler{threshold: slog.LevelError, queue: q}
			rec := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)

			done := make(chan error, 1)
			go func() { done <- h.Handle(context.Background(), rec) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Handle should return nil when dropping, got: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("Handle blocked on a full queue")
			}
		})
	}
}

func TestOpenActionLogValue(t *testing.T) {
	a := openAction{label: "Show output", path: "/tmp/x"}
	if got := a.LogValue().String(); got != "/tmp/x" {
		t.Errorf("LogValue() = %q, want %q", got, "/tmp/x")
	}
}

func TestNotifierHandleSignal(t *testing.T) {
	origOpen := openTarget
	defer func() { openTarget = origOpen }()

	var opened []string
	openTarget = func(path string) error {
		opened = append(opened, path)
		return nil
	}

	n := &notifier{pending: map[uint32]string{7: "/tmp/known"}}

	// Unknown ID: no action taken.
	n.handleSignal(notifyBusName+".ActionInvoked", []interface{}{uint32(99), "default"})
	if len(opened) != 0 {
		t.Fatalf("expected no calls for unknown notification id, got %v", opened)
	}

	// Known ID: openTarget called with the recorded path.
	n.handleSignal(notifyBusName+".ActionInvoked", []interface{}{uint32(7), "default"})
	if len(opened) != 1 || opened[0] != "/tmp/known" {
		t.Fatalf("expected openTarget(\"/tmp/known\"), got %v", opened)
	}

	// NotificationClosed removes the id from pending.
	n.handleSignal(notifyBusName+".NotificationClosed", []interface{}{uint32(7), uint32(1)})
	n.mu.Lock()
	_, stillPending := n.pending[7]
	n.mu.Unlock()
	if stillPending {
		t.Error("expected id 7 to be removed from pending after NotificationClosed")
	}
}
