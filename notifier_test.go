package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"pausepoint/internal/breaks"
)

type testNotifier struct {
	onShow    func(context.Context, breakNotice, noticeReceipt) (noticeReceipt, error)
	onDismiss func(context.Context, noticeReceipt) error
}

func (n testNotifier) show(ctx context.Context, notice breakNotice, previous noticeReceipt) (noticeReceipt, error) {
	return n.onShow(ctx, notice, previous)
}

func (n testNotifier) dismiss(ctx context.Context, receipt noticeReceipt) error {
	return n.onDismiss(ctx, receipt)
}

func (testNotifier) close() {}

func awaitNoticeTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("notification worker did not respond")
		var zero T
		return zero
	}
}

func TestSlowNoticeDoesNotBlockBreakScheduling(t *testing.T) {
	a := newTestDesktop(t)
	showStarted := make(chan struct{})
	allowShow := make(chan struct{})
	dismissStarted := make(chan noticeReceipt, 1)
	allowDismiss := make(chan struct{})
	a.notifications = newNoticeWorker(testNotifier{
		onShow: func(_ context.Context, _ breakNotice, _ noticeReceipt) (noticeReceipt, error) {
			close(showStarted)
			<-allowShow // Simulate a daemon that returns an ID after cancellation.
			return uint32(42), nil
		},
		onDismiss: func(_ context.Context, receipt noticeReceipt) error {
			dismissStarted <- receipt
			<-allowDismiss
			return nil
		},
	}, nil)
	t.Cleanup(func() {
		close(allowDismiss)
		a.notifications.stop()
		awaitNoticeTest(t, a.notifications.done)
	})
	var releaseShow sync.Once
	t.Cleanup(func() { releaseShow.Do(func() { close(allowShow) }) })

	now := time.Now()
	a.scheduler = breaks.New(a.prefs.Settings, now)
	deadline := now.Add(a.prefs.Interval)
	a.tick(deadline.Add(-a.prefs.Short.Notice))
	awaitNoticeTest(t, showStarted)

	// Keep the fake transport blocked until the caller returns from both ticks.
	resting := make(chan breaks.State, 1)
	go func() {
		a.tick(deadline)
		a.tick(deadline.Add(time.Second))
		resting <- a.state
	}()
	if state := awaitNoticeTest(t, resting); state.Phase != breaks.Active || state.Remaining != a.prefs.Short.Duration-time.Second {
		t.Fatalf("slow notification blocked or changed the break: %+v", state)
	}
	releaseShow.Do(func() { close(allowShow) })
	if receipt := awaitNoticeTest(t, dismissStarted); receipt != uint32(42) {
		t.Fatalf("dismissed receipt %v, want the late receipt 42", receipt)
	}
	go func() {
		a.tick(deadline.Add(a.prefs.Short.Duration))
		resting <- a.state
	}()
	if state := awaitNoticeTest(t, resting); state.Phase != breaks.Waiting {
		t.Fatalf("slow dismissal blocked the next interval: %+v", state)
	}
}

func TestNoticeTimeoutAndStaleFailureKeepSchedulerRunning(t *testing.T) {
	a := newTestDesktop(t)
	failures := make(chan *breakNotice, 1)
	a.notifications = newNoticeWorker(testNotifier{
		onShow: func(context.Context, breakNotice, noticeReceipt) (noticeReceipt, error) {
			return nil, fmt.Errorf("test D-Bus timeout: %w", context.DeadlineExceeded)
		},
		onDismiss: func(context.Context, noticeReceipt) error {
			return nil
		},
	}, func(notice *breakNotice) { failures <- notice })
	t.Cleanup(func() {
		a.notifications.stop()
		awaitNoticeTest(t, a.notifications.done)
	})

	now := time.Now()
	a.scheduler = breaks.New(a.prefs.Settings, now)
	deadline := now.Add(a.prefs.Interval)
	a.tick(deadline.Add(-a.prefs.Short.Notice))
	failed := awaitNoticeTest(t, failures)
	a.noticeFailed(failed)
	if !a.notifyFailed || a.state.Phase != breaks.Waiting {
		t.Fatalf("current failure should report degraded notifications only: %+v", a.state)
	}
	a.tick(deadline)
	a.notifyFailed = false
	a.noticeFailed(failed)
	if a.notifyFailed || a.state.Phase != breaks.Active || a.notification != nil {
		t.Fatalf("stale failure changed the active break: %+v", a.state)
	}
}

func TestNoticeWorkerConcurrentUpdatesAndDismissals(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	var first sync.Once
	w := newNoticeWorker(testNotifier{
		onShow: func(ctx context.Context, _ breakNotice, _ noticeReceipt) (noticeReceipt, error) {
			first.Do(func() {
				close(started)
				<-ctx.Done()
				close(canceled)
			})
			return nil, ctx.Err()
		},
		onDismiss: func(context.Context, noticeReceipt) error { return nil },
	}, nil)
	t.Cleanup(func() {
		w.stop()
		awaitNoticeTest(t, w.done)
	})
	expires := time.Now().Add(time.Minute)
	w.update(&breakNotice{expires: expires})
	awaitNoticeTest(t, started)
	var updates sync.WaitGroup
	for range 8 {
		updates.Go(func() {
			for range 32 {
				w.update(&breakNotice{expires: expires})
				w.update(nil)
			}
		})
	}
	updates.Wait()
	awaitNoticeTest(t, canceled)
}
