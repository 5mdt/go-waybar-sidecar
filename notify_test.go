package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestUrgencyForLevel(t *testing.T) {
	cases := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelError, "critical"},
		{slog.LevelWarn, "normal"},
		{slog.LevelInfo, "low"},
		{slog.LevelDebug, "low"},
	}
	for _, c := range cases {
		if got := urgencyForLevel(c.level); got != c.want {
			t.Errorf("urgencyForLevel(%v) = %q, want %q", c.level, got, c.want)
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

func TestNotifyHandlerHandle(t *testing.T) {
	origSend := sendNotification
	defer func() { sendNotification = origSend }()

	type call struct {
		app, urgency, summary, body string
	}
	var calls []call
	sendNotification = func(app, urgency, summary, body string) error {
		calls = append(calls, call{app, urgency, summary, body})
		return nil
	}

	h := &notifyHandler{threshold: slog.LevelWarn, appName: "go-waybar-sidecar"}

	below := slog.NewRecord(time.Now(), slog.LevelInfo, "below threshold", 0)
	if err := h.Handle(context.Background(), below); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("expected no calls for below-threshold record, got %v", calls)
	}

	rec := slog.NewRecord(time.Now(), slog.LevelError, "waybar failed to start", 0)
	rec.AddAttrs(slog.String("err", "boom"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	got := calls[0]
	if got.app != "go-waybar-sidecar" {
		t.Errorf("app = %q, want %q", got.app, "go-waybar-sidecar")
	}
	if got.urgency != "critical" {
		t.Errorf("urgency = %q, want %q", got.urgency, "critical")
	}
	if got.summary != "waybar failed to start" {
		t.Errorf("summary = %q, want %q", got.summary, "waybar failed to start")
	}
	if !strings.Contains(got.body, "err=boom") {
		t.Errorf("body = %q, want it to contain %q", got.body, "err=boom")
	}
}

func TestNotifyHandlerHandleSwallowsErrors(t *testing.T) {
	origSend := sendNotification
	defer func() { sendNotification = origSend }()

	sendNotification = func(app, urgency, summary, body string) error {
		return errors.New("notify-send: command not found")
	}

	h := &notifyHandler{threshold: slog.LevelError, appName: "go-waybar-sidecar"}
	rec := slog.NewRecord(time.Now(), slog.LevelError, "boom", 0)

	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle should swallow sendNotification errors, got: %v", err)
	}
}

func TestNotifyAvailable(t *testing.T) {
	origLook := lookNotifySend
	defer func() { lookNotifySend = origLook }()

	lookNotifySend = func() bool { return false }
	if notifyAvailable() {
		t.Error("expected notifyAvailable=false when notify-send is missing")
	}

	lookNotifySend = func() bool { return true }
	if !notifyAvailable() {
		t.Error("expected notifyAvailable=true when notify-send is present")
	}
}
