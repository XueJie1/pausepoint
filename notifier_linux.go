//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	notificationName = "org.freedesktop.Notifications"
	notificationPath = dbus.ObjectPath("/org/freedesktop/Notifications")
)

type linuxBreakNotifier struct {
	bus    *dbus.Conn
	socket *net.UnixConn
}

type linuxNoticeReceipt struct {
	bus   *dbus.Conn
	owner string
	id    uint32
}

func newBreakNotifier() breakNotifier { return &linuxBreakNotifier{} }

func (n *linuxBreakNotifier) show(ctx context.Context, notice breakNotice, previous noticeReceipt) (noticeReceipt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A sent D-Bus call cannot be canceled at the daemon. Keep waiting for its
	// ID within the transport budget so the worker can close a superseded notice.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), notificationTimeout)
	defer cancel()
	if err := n.connect(ctx); err != nil {
		return nil, err
	}
	owner, err := n.notificationOwner(ctx)
	if err != nil {
		return nil, err
	}
	old, hasPrevious := previous.(linuxNoticeReceipt)
	var replaces uint32
	if hasPrevious && old.bus == n.bus && old.owner == owner {
		replaces = old.id
	}
	expires := int32(-1)
	if !notice.expires.IsZero() {
		remaining := time.Until(notice.expires)
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		expires = int32(min(math.MaxInt32, max(1, remaining.Milliseconds())))
	}
	var id uint32
	err = n.call(ctx, n.bus.Object(owner, notificationPath), notificationName+".Notify",
		"Pausepoint", replaces, "", notice.title, notice.body, notice.actions, map[string]dbus.Variant{}, expires).Store(&id)
	if err != nil {
		return nil, fmt.Errorf("send desktop notification: %w", err)
	}
	receipt := linuxNoticeReceipt{bus: n.bus, owner: owner, id: id}
	if hasPrevious && old.bus == n.bus && old.owner != owner {
		// The old daemon may still be running after relinquishing its name.
		// It cannot be replaced through the new owner, so close it separately.
		_ = n.dismiss(ctx, old)
	}
	return receipt, nil
}

func (n *linuxBreakNotifier) dismiss(ctx context.Context, receipt noticeReceipt) error {
	old, ok := receipt.(linuxNoticeReceipt)
	if !ok || old.bus != n.bus || !n.bus.Connected() {
		return nil
	}
	// Address the unique owner so an ID from an old notification daemon cannot
	// accidentally dismiss a notification after the daemon restarts.
	err := n.call(ctx, n.bus.Object(old.owner, notificationPath), notificationName+".CloseNotification", old.id).Err
	var busErr dbus.Error
	if errors.As(err, &busErr) && (busErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner" || busErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown") {
		return nil
	}
	return err
}

func (n *linuxBreakNotifier) close() {
	if n.bus != nil {
		_ = n.bus.Close()
		n.bus, n.socket = nil, nil
	}
}

func (n *linuxBreakNotifier) call(ctx context.Context, target dbus.BusObject, method string, args ...any) *dbus.Call {
	// CallWithContext bounds reply waiting. A socket deadline also bounds the
	// synchronous write in Godbus if the session bus stops draining its socket.
	deadline, _ := ctx.Deadline()
	if err := n.socket.SetWriteDeadline(deadline); err != nil {
		n.close()
		return &dbus.Call{Err: err}
	}
	call := target.CallWithContext(ctx, method, 0, args...)
	var socketErr *net.OpError
	if errors.As(call.Err, &socketErr) || errors.Is(call.Err, dbus.ErrClosed) {
		n.close()
	}
	return call
}

func (n *linuxBreakNotifier) notificationOwner(ctx context.Context) (string, error) {
	bus := n.bus.BusObject()
	var owner string
	err := n.call(ctx, bus, "org.freedesktop.DBus.GetNameOwner", notificationName).Store(&owner)
	var busErr dbus.Error
	if errors.As(err, &busErr) && busErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner" {
		if err = n.call(ctx, bus, "org.freedesktop.DBus.StartServiceByName", notificationName, uint32(0)).Err; err == nil {
			err = n.call(ctx, bus, "org.freedesktop.DBus.GetNameOwner", notificationName).Store(&owner)
		}
	}
	if err != nil {
		return "", fmt.Errorf("find desktop notification service: %w", err)
	}
	return owner, nil
}

func (n *linuxBreakNotifier) connect(ctx context.Context) error {
	if n.bus != nil && n.bus.Connected() {
		return nil
	}
	n.close()
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" {
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			address = "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(runtimeDir, "bus"))
		}
	}
	var failures error
	for endpoint := range strings.SplitSeq(address, ";") {
		socketName, ok := sessionBusSocket(endpoint)
		if !ok {
			continue
		}
		socket, bus, err := connectNotificationBus(ctx, socketName)
		if err == nil {
			n.socket, n.bus = socket, bus
			return nil
		}
		failures = errors.Join(failures, err)
		if ctx.Err() != nil {
			break
		}
	}
	if failures == nil {
		failures = errors.New("session bus has no usable Unix socket address")
	}
	return fmt.Errorf("connect to desktop notification bus: %w", failures)
}

func connectNotificationBus(ctx context.Context, name string) (*net.UnixConn, *dbus.Conn, error) {
	socket, err := (&net.Dialer{}).DialContext(ctx, "unix", name)
	if err != nil {
		return nil, nil, err
	}
	unixSocket := socket.(*net.UnixConn)
	deadline, _ := ctx.Deadline()
	if err := unixSocket.SetDeadline(deadline); err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	bus, err := dbus.ConnectUnix(unixSocket)
	if err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	if err := unixSocket.SetDeadline(time.Time{}); err != nil {
		_ = bus.Close()
		return nil, nil, err
	}
	return unixSocket, bus, nil
}

func sessionBusSocket(endpoint string) (string, bool) {
	keys, ok := strings.CutPrefix(endpoint, "unix:")
	if !ok {
		return "", false
	}
	var name string
	for option := range strings.SplitSeq(keys, ",") {
		key, raw, found := strings.Cut(option, "=")
		if !found || key != "path" && key != "abstract" {
			continue
		}
		value, err := dbus.UnescapeBusAddressValue(raw)
		if err != nil || value == "" || name != "" {
			return "", false
		}
		if key == "abstract" {
			value = "\x00" + value
		}
		name = value
	}
	return name, name != ""
}
