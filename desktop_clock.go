package main

import (
	"time"

	"github.com/egoist/mygo"

	"pausepoint/internal/breaks"
)

const (
	tickInterval  = 250 * time.Millisecond
	focusInterval = time.Second
)

// nextTick is independent of the event loop. A faster cadence never rounds
// away a scheduler, preview, or autosave deadline.
func (a *desktop) nextTick(now time.Time, settingsVisible bool) time.Time {
	deadline, inNotice := a.scheduler.NextDeadline()
	state := a.scheduler.State(now)
	var next time.Time
	if settingsVisible || !a.previewUntil.IsZero() || !a.saveAfter.IsZero() || state.Phase == breaks.Active || inNotice {
		next = now.Add(tickInterval)
	} else if state.Phase == breaks.Waiting {
		// The display rounds remaining time up. Wake when that value next
		// changes, relative to the break deadline rather than wall time.
		delay := state.Remaining % focusInterval
		if delay == 0 {
			delay = focusInterval
		}
		next = now.Add(delay)
	}
	for _, at := range [...]time.Time{deadline, a.previewUntil} {
		if !at.IsZero() && (next.IsZero() || at.Before(next)) {
			next = at
		}
	}
	if !a.settingsLocked() && !a.saveAfter.IsZero() && (next.IsZero() || a.saveAfter.Before(next)) {
		next = a.saveAfter
	}
	return next
}

// scheduleTick and the callback's desktop state checks run on the UI thread.
func (a *desktop) scheduleTick() {
	if !a.ticksRunning || a.quitting {
		return
	}
	next := a.nextTick(time.Now(), a.settings != nil && a.settings.IsVisible())
	if next.Equal(a.tickAt) {
		return
	}
	a.stopTick()
	if next.IsZero() {
		return
	}
	a.tickAt = next
	a.tickTimer = time.AfterFunc(time.Until(next), func() {
		mygo.RunOnMain(func() {
			// A UI event may have replaced this timer while it was in flight.
			if !a.tickAt.Equal(next) || a.quitting {
				return
			}
			a.tickTimer = nil
			a.tickAt = time.Time{}
			a.tick(time.Now())
		})
	})
}

func (a *desktop) stopTick() {
	if a.tickTimer != nil {
		a.tickTimer.Stop()
		a.tickTimer = nil
	}
	a.tickAt = time.Time{}
}
