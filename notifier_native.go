//go:build !linux

package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
)

func newBreakNotifier() breakNotifier { return &nativeBreakNotifier{} }

type nativeBreakNotifier struct {
	// The worker owns notification delivery. The quit hook can close the
	// native notification before MyGo stops dispatching main thread work.
	mu      sync.Mutex
	current *mygo.Notification
	closed  bool
}

func (n *nativeBreakNotifier) show(ctx context.Context, notice breakNotice, previous noticeReceipt) (noticeReceipt, error) {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil, context.Canceled
	}
	if err := ctx.Err(); err != nil {
		n.mu.Unlock()
		return nil, err
	}
	opts := mygo.NotificationOptions{Title: notice.title, Body: notice.body}
	if old, ok := previous.(*mygo.Notification); ok {
		opts.ID = old.ID()
	}
	note := mygo.NewNotification(opts)
	n.current = note
	n.mu.Unlock()
	// Keep the receipt even on cancellation so dismissal can remove both a
	// delivered notification and a pending macOS authorization request.
	return note, note.ShowContext(ctx)
}

func (n *nativeBreakNotifier) dismiss(_ context.Context, receipt noticeReceipt) error {
	note, ok := receipt.(*mygo.Notification)
	if !ok {
		return nil
	}
	mygo.RunOnMain(func() {
		n.mu.Lock()
		current := n.current == note
		if current {
			n.current = nil
		}
		n.mu.Unlock()
		if current {
			note.Close()
		}
	})
	return nil
}

func (n *nativeBreakNotifier) close() { mygo.RunOnMain(n.closeOnQuit) }

// closeOnQuit runs on MyGo's main thread after the notification worker is told
// to stop. It does not wait for the worker or a pending authorization answer.
func (n *nativeBreakNotifier) closeOnQuit() {
	n.mu.Lock()
	n.closed = true
	note := n.current
	n.current = nil
	n.mu.Unlock()
	if note != nil {
		note.Close()
	}
}
