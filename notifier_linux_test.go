//go:build linux

package main

import (
	"bufio"
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestSessionBusSocket(t *testing.T) {
	for _, tc := range []struct{ address, want string }{
		{"unix:path=/tmp/bus%2Ctest,guid=abcd", "/tmp/bus,test"},
		{"unix:abstract=bus%2Dtest", "\x00bus-test"},
		{"unix:path=/tmp/bus,abstract=duplicate", ""},
		{"unix:path=%ZZ", ""},
		{"tcp:host=localhost,port=1234", ""},
	} {
		got, ok := sessionBusSocket(tc.address)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("parse %q = %q, %t, want %q", tc.address, got, ok, tc.want)
		}
	}
}

func TestLinuxNotifierBoundsStalledBusHandshake(t *testing.T) {
	name := filepath.Join(t.TempDir(), "bus")
	listener, err := net.Listen("unix", name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	// The socket accepts connects, but the peer never responds to D-Bus Auth.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	socket, bus, err := connectNotificationBus(ctx, name)
	if err == nil || socket != nil || bus != nil {
		t.Fatalf("connect succeeded against a stalled bus: %v", err)
	}
	if ctx.Err() == nil {
		// A socket deadline can wake the waiter just before the context timer.
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("handshake returned before the timeout: %v", err)
		}
	}
}

type linuxNotifyRequest struct {
	app, title, body string
	replaces         uint32
	actions          []string
	expires          int32
}

type testNotificationDaemon struct {
	requests    chan linuxNotifyRequest
	closed      chan uint32
	id          uint32
	beforeReply func()
}

func (d *testNotificationDaemon) Notify(app string, replaces uint32, _ string, title, body string, actions []string, _ map[string]dbus.Variant, expires int32) (uint32, *dbus.Error) {
	d.requests <- linuxNotifyRequest{app: app, replaces: replaces, title: title, body: body, actions: actions, expires: expires}
	if d.beforeReply != nil {
		d.beforeReply()
	}
	return d.id, nil
}

func (d *testNotificationDaemon) CloseNotification(id uint32) *dbus.Error {
	d.closed <- id
	return nil
}

func TestLinuxNotifierTracksReplacementAndDaemonOwner(t *testing.T) {
	address := startTestNotificationBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)

	firstConn, first := startTestNotificationDaemon(t, address, nil)
	n := &linuxBreakNotifier{}
	defer n.close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	note := breakNotice{title: "Break reminder", body: "Rest soon", actions: []string{"default", "Open"}, expires: time.Now().Add(time.Minute)}
	receipt, err := n.show(ctx, note, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := <-first.requests
	if req.app != "Pausepoint" || req.title != note.title || req.body != note.body || req.replaces != 0 || !reflect.DeepEqual(req.actions, note.actions) || req.expires <= 0 || req.expires > 60000 {
		t.Fatalf("first request = %+v", req)
	}
	receipt, err = n.show(ctx, note, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if req = <-first.requests; req.replaces != first.id {
		t.Fatalf("replace ID = %d, want %d", req.replaces, first.id)
	}

	if _, err := firstConn.ReleaseName(notificationName); err != nil {
		t.Fatal(err)
	}
	_, second := startTestNotificationDaemon(t, address, nil)
	current, err := n.show(ctx, note, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if req = <-second.requests; req.replaces != 0 {
		t.Fatalf("old daemon ID leaked into replacement request: %d", req.replaces)
	}
	select {
	case id := <-first.closed:
		if id != first.id {
			t.Fatalf("closed old notification %d, want %d", id, first.id)
		}
	default:
		t.Fatal("replacement did not close the notification on the previous daemon")
	}
	select {
	case id := <-second.closed:
		t.Fatalf("old receipt closed another daemon's notification %d", id)
	default:
	}
	if err := n.dismiss(ctx, current); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-second.closed:
		if id != second.id {
			t.Fatalf("closed new notification %d, want %d", id, second.id)
		}
	default:
		t.Fatal("new receipt did not address the new daemon")
	}
}

func TestLinuxNotifierReceivesIDAfterCancellation(t *testing.T) {
	address := startTestNotificationBus(t)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, daemon := startTestNotificationDaemon(t, address, cancel)
	n := &linuxBreakNotifier{}
	defer n.close()

	receipt, err := n.show(ctx, breakNotice{title: "Break reminder", expires: time.Now().Add(time.Minute)}, nil)
	if err != nil || ctx.Err() == nil {
		t.Fatalf("did not collect the reply after cancellation: %v", err)
	}
	dismissCtx, stopDismiss := context.WithTimeout(context.Background(), notificationTimeout)
	defer stopDismiss()
	if err := n.dismiss(dismissCtx, receipt); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-daemon.closed:
		if id != daemon.id {
			t.Fatalf("closed ID = %d, want %d", id, daemon.id)
		}
	default:
		t.Fatal("stale notification was not closed")
	}
}

func startTestNotificationDaemon(t *testing.T, address string, beforeReply func()) (*dbus.Conn, *testNotificationDaemon) {
	t.Helper()
	conn, err := dbus.Connect(address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	d := &testNotificationDaemon{requests: make(chan linuxNotifyRequest, 4), closed: make(chan uint32, 4), id: 42, beforeReply: beforeReply}
	if err := conn.Export(d, notificationPath, notificationName); err != nil {
		t.Fatal(err)
	}
	result, err := conn.RequestName(notificationName, dbus.NameFlagDoNotQueue)
	if err != nil || result != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("acquire notifications bus name: result %v, error %v", result, err)
	}
	return conn, d
}

func startTestNotificationBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon required for private session-bus test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	address := "unix:path=" + dbus.EscapeBusAddressValue(filepath.Join(t.TempDir(), "bus"))
	cmd := exec.CommandContext(ctx, daemon, "--session", "--nofork", "--print-address=1", "--address="+address)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	out := bufio.NewScanner(stdout)
	if !out.Scan() {
		t.Fatalf("private D-Bus daemon did not print its address: %v", out.Err())
	}
	return out.Text()
}
