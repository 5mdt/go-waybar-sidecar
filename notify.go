package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

const notifyAppName = "go-waybar-sidecar"

// sendNotification shells out to notify-send. Overridable for testing.
var sendNotification = func(app, urgency, summary, body string) error {
	args := []string{"-a", app, "-u", urgency, summary}
	if body != "" {
		args = append(args, body)
	}
	return exec.Command("notify-send", args...).Run()
}

// lookNotifySend reports whether notify-send is available on PATH.
// Overridable for testing.
var lookNotifySend = func() bool {
	_, err := exec.LookPath("notify-send")
	return err == nil
}

// notifyAvailable reports whether desktop notifications can be delivered.
func notifyAvailable() bool {
	return lookNotifySend()
}

// urgencyForLevel maps a slog level to a notify-send urgency.
func urgencyForLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "critical"
	case level >= slog.LevelWarn:
		return "normal"
	default:
		return "low"
	}
}

// parseNotifyLevel parses the --notify-level flag value. "off" disables
// notifications (enabled=false); any other valid slog level enables them.
func parseNotifyLevel(s string) (level slog.Level, enabled bool, err error) {
	if strings.EqualFold(s, "off") {
		return level, false, nil
	}
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return level, false, err
	}
	return level, true, nil
}

// notifyHandler is a slog.Handler that forwards records at or above its
// threshold to notify-send. It is best-effort: any failure to send a
// notification is swallowed, never returned, so logging is unaffected by
// a missing or broken notify-send.
type notifyHandler struct {
	threshold slog.Level
	appName   string
	attrs     []slog.Attr
}

func (h *notifyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.threshold
}

func (h *notifyHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < h.threshold {
		return nil
	}

	var body strings.Builder
	for _, a := range h.attrs {
		if body.Len() > 0 {
			body.WriteByte(' ')
		}
		fmt.Fprintf(&body, "%s=%v", a.Key, a.Value)
	}
	r.Attrs(func(a slog.Attr) bool {
		if body.Len() > 0 {
			body.WriteByte(' ')
		}
		fmt.Fprintf(&body, "%s=%v", a.Key, a.Value)
		return true
	})

	_ = sendNotification(h.appName, urgencyForLevel(r.Level), r.Message, body.String())
	return nil
}

func (h *notifyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &notifyHandler{
		threshold: h.threshold,
		appName:   h.appName,
		attrs:     append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

func (h *notifyHandler) WithGroup(_ string) slog.Handler {
	return h
}

// multiHandler fans out log records to multiple handlers, each gated on its
// own Enabled.
type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}
