package main

import (
	"context"
	"log"
	"sync"
	"time"
)

const notificationTimeout = 1750 * time.Millisecond

type breakNotice struct {
	title, body string
	expires     time.Time
	actions     []string // alternating action IDs and labels
}

type noticeReceipt = any

// breakNotifier is used exclusively by the notification worker. A replacement
// can reuse the previous receipt. On failure, the worker retains it unless the
// backend returns a new receipt for cleanup.
type breakNotifier interface {
	show(context.Context, breakNotice, noticeReceipt) (noticeReceipt, error)
	dismiss(context.Context, noticeReceipt) error
	close()
}

// noticeWorker keeps just the latest desired notice. The scheduler and UI can
// publish changes while the worker is waiting on a native notification service.
type noticeWorker struct {
	backend    breakNotifier
	onFailure  func(*breakNotice)
	wake, done chan struct{}

	mu      sync.Mutex
	desired *breakNotice
	cancel  context.CancelFunc
	stopped bool
}

func newNoticeWorker(backend breakNotifier, onFailure func(*breakNotice)) *noticeWorker {
	w := &noticeWorker{backend: backend, onFailure: onFailure, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go w.run()
	return w
}

func (w *noticeWorker) update(notice *breakNotice) {
	w.set(notice, false)
}

func (w *noticeWorker) stop() {
	w.set(nil, true)
}

func (w *noticeWorker) set(notice *breakNotice, stop bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || (!stop && w.desired == notice) {
		return
	}
	w.desired, w.stopped = notice, stop
	if w.cancel != nil {
		w.cancel()
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *noticeWorker) run() {
	defer close(w.done)
	defer w.backend.close()
	var processed *breakNotice
	var displayed noticeReceipt
	for range w.wake {
		w.mu.Lock()
		notice, stopped := w.desired, w.stopped
		if !stopped && notice == processed {
			w.mu.Unlock()
			continue
		}
		processed = notice
		if notice == nil || !time.Now().Before(notice.expires) {
			w.mu.Unlock()
			w.dismiss(displayed)
			displayed = nil
			if stopped {
				return
			}
			continue
		}
		ctx, cancel := context.WithDeadline(context.Background(), notice.expires)
		w.cancel = cancel
		w.mu.Unlock()

		receipt, err := w.backend.show(ctx, *notice, displayed)
		if receipt != nil {
			displayed = receipt
		}
		w.mu.Lock()
		current := w.desired == notice
		w.cancel = nil
		w.mu.Unlock()
		live := ctx.Err() == nil
		cancel()
		if err != nil && current && live {
			log.Print("advance notice: ", err)
			if w.onFailure != nil {
				w.onFailure(notice)
			}
		}
	}
}

func (w *noticeWorker) dismiss(receipt noticeReceipt) {
	if receipt == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), notificationTimeout)
	defer cancel()
	if err := w.backend.dismiss(ctx, receipt); err != nil {
		log.Print("dismiss advance notice: ", err)
	}
}
