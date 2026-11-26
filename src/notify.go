package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const notifyAppName = "go-waybar-sidecar"

// notifyQueueSize bounds notifications waiting for the sender goroutine.
const notifyQueueSize = 16

// notifyActionKey is the slog attribute key a call site uses to override a
// notification's default click action. See notifyOpen.
const notifyActionKey = "notify_open"

const (
	notifyBusName    = "org.freedesktop.Notifications"
	notifyObjectPath = "/org/freedesktop/Notifications"
)

// openAction is the value of a notifyActionKey attribute: what a
// notification's click action should open. It implements slog.LogValuer so
// text logs show only the path - the label is notification-only.
type openAction struct {
	label string
	path  string
}

func (a openAction) LogValue() slog.Value { return slog.StringValue(a.path) }

// notifyOpen returns a slog.Attr that overrides a notification's default
// click action ("Open config") to open label/path with xdg-open instead.
func notifyOpen(label, path string) slog.Attr {
	return slog.Any(notifyActionKey, openAction{label: label, path: path})
}

// notification is one queued desktop notification with a single click action.
type notification struct {
	urgency       byte
	summary, body string
	label, path   string
}

// openTarget fulfils a clicked notification action. Overridable for testing.
var openTarget = func(path string) error {
	return exec.Command("xdg-open", path).Start()
}

// notifier owns a session-bus connection to the freedesktop notifications
// server and tracks which notification IDs are still awaiting a click.
type notifier struct {
	conn    *dbus.Conn
	obj     dbus.BusObject
	appName string

	mu      sync.Mutex
	pending map[uint32]string
}

// newNotifier connects to the session bus and verifies the notification
// server supports actions. ok is false if either step fails, meaning
// notifications should stay text-only.
func newNotifier(appName string) (n *notifier, ok bool) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, false
	}

	obj := conn.Object(notifyBusName, dbus.ObjectPath(notifyObjectPath))

	var caps []string
	if err := obj.Call(notifyBusName+".GetCapabilities", 0).Store(&caps); err != nil {
		return nil, false
	}
	if !slices.Contains(caps, "actions") {
		return nil, false
	}

	n = &notifier{conn: conn, obj: obj, appName: appName, pending: make(map[uint32]string)}
	n.listen()
	return n, true
}

// listen starts a goroutine that receives ActionInvoked/NotificationClosed
// signals for the lifetime of the connection.
func (n *notifier) listen() {
	ch := make(chan *dbus.Signal, 16)
	n.conn.Signal(ch)
	_ = n.conn.AddMatchSignal(dbus.WithMatchInterface(notifyBusName))

	go func() {
		for sig := range ch {
			n.handleSignal(sig.Name, sig.Body)
		}
	}()
}

// handleSignal dispatches a single ActionInvoked/NotificationClosed signal.
// Split out from listen so it can be tested without a real bus connection.
func (n *notifier) handleSignal(name string, body []interface{}) {
	if len(body) < 1 {
		return
	}
	id, ok := body[0].(uint32)
	if !ok {
		return
	}

	switch name {
	case notifyBusName + ".ActionInvoked":
		n.mu.Lock()
		path, found := n.pending[id]
		n.mu.Unlock()
		if found {
			_ = openTarget(path)
		}
	case notifyBusName + ".NotificationClosed":
		n.mu.Lock()
		delete(n.pending, id)
		n.mu.Unlock()
	}
}

// send calls Notify with a single "default" action and records the
// resulting notification ID against path so a later click can resolve it.
func (n *notifier) send(m notification) error {
	actions := []string{"default", m.label}
	hints := map[string]dbus.Variant{"urgency": dbus.MakeVariant(m.urgency)}

	call := n.obj.Call(notifyBusName+".Notify", 0,
		n.appName, uint32(0), "", m.summary, m.body, actions, hints, int32(-1))
	if call.Err != nil {
		return call.Err
	}

	var id uint32
	if err := call.Store(&id); err != nil {
		return err
	}

	n.mu.Lock()
	n.pending[id] = m.path
	n.mu.Unlock()
	return nil
}

// urgencyHint maps a slog level to the Notify "urgency" hint byte.
func urgencyHint(level slog.Level) byte {
	switch {
	case level >= slog.LevelError:
		return 2
	case level >= slog.LevelWarn:
		return 1
	default:
		return 0
	}
}

// parseNotifyLevel parses the --notify-level flag value. "off" disables
// notifications (enabled=false); any other valid slog level enables them.
func parseNotifyLevel(s string) (level slog.Level, enabled bool, err error) {
	if strings.EqualFold(s, "off") {
		return level, false, nil
	}
	level, err = parseLogLevel(s)
	return level, err == nil, err
}

// notifyHandler is a slog.Handler that queues records at or above its
// threshold for a sender goroutine. It is best-effort: when the queue is full
// the record is dropped, never blocked on or returned as an error, so logging
// is unaffected by a missing, slow or broken notification daemon.
type notifyHandler struct {
	threshold slog.Level
	queue     chan<- notification
	attrs     []slog.Attr
}

func (h *notifyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.threshold
}

func (h *notifyHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < h.threshold {
		return nil
	}

	label, path := "Open config", configPath
	var body strings.Builder

	appendAttr := func(a slog.Attr) {
		if a.Key == notifyActionKey {
			if act, ok := a.Value.Any().(openAction); ok {
				label, path = act.label, act.path
			}
			return
		}
		if body.Len() > 0 {
			body.WriteByte(' ')
		}
		fmt.Fprintf(&body, "%s=%v", a.Key, a.Value)
	}

	for _, a := range h.attrs {
		appendAttr(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(a)
		return true
	})

	select {
	case h.queue <- notification{urgencyHint(r.Level), r.Message, body.String(), label, path}:
	default:
	}
	return nil
}

func (h *notifyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &notifyHandler{
		threshold: h.threshold,
		queue:     h.queue,
		attrs:     append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

func (h *notifyHandler) WithGroup(_ string) slog.Handler {
	return h
}
