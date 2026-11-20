package main

import (
	"testing"
	"time"

	"pausepoint/internal/breaks"
	"pausepoint/internal/prefs"
)

func TestFocusClockKeepsSchedulerDeadlines(t *testing.T) {
	a := newTestDesktop(t)
	start := time.Unix(1000, 123000000)
	rules := a.prefs.Settings
	rules.Interval = 3*time.Second + 100*time.Millisecond
	rules.Short.Notice = 550 * time.Millisecond
	rules.Short.Duration = 650 * time.Millisecond
	a.scheduler = breaks.New(rules, start)
	a.state = a.scheduler.State(start)

	previous := start
	for _, step := range []struct {
		offset time.Duration
		phase  breaks.Phase
	}{
		{100 * time.Millisecond, breaks.Waiting}, // Align to the displayed second, not wall time.
		{1100 * time.Millisecond, breaks.Waiting},
		{2100 * time.Millisecond, breaks.Waiting},
		{2550 * time.Millisecond, breaks.Waiting}, // Advance notice between whole seconds.
		{2800 * time.Millisecond, breaks.Waiting},
		{3050 * time.Millisecond, breaks.Waiting},
		{3100 * time.Millisecond, breaks.Active}, // Break starts between fast ticks.
		{3350 * time.Millisecond, breaks.Active},
		{3600 * time.Millisecond, breaks.Active},
		{3750 * time.Millisecond, breaks.Waiting}, // Break ends between fast ticks.
		{3850 * time.Millisecond, breaks.Waiting}, // Back to focus alignment.
		{4850 * time.Millisecond, breaks.Waiting},
	} {
		want := start.Add(step.offset)
		if got := a.nextTick(previous, false); !got.Equal(want) {
			t.Fatalf("next wake from %v = %v, want %v", previous.Sub(start), got.Sub(start), step.offset)
		}
		a.tick(want)
		if a.state.Phase != step.phase {
			t.Fatalf("phase at %v = %v, want %v", step.offset, a.state.Phase, step.phase)
		}
		previous = want
	}
}

func TestDesktopClockSettingsAndAutosave(t *testing.T) {
	a := newTestDesktop(t)
	start := time.Unix(2000, 123000000)
	a.scheduler = breaks.New(a.prefs.Settings, start)
	a.state = a.scheduler.State(start)

	check := func(now time.Time, visible bool, delay time.Duration) {
		t.Helper()
		if got := a.nextTick(now, visible); !got.Equal(now.Add(delay)) {
			t.Fatalf("next wake at %v (settings visible: %t) after %v, want %v", now.Sub(start), visible, got.Sub(now), delay)
		}
	}
	check(start, false, time.Second)
	check(start, true, 250*time.Millisecond)
	a.previewUntil = start.Add(150 * time.Millisecond)
	check(start, false, 150*time.Millisecond)
	a.tick(a.previewUntil)
	check(start.Add(150*time.Millisecond), false, 850*time.Millisecond)

	a.draft.interval = "900"
	a.settingsPending = true
	a.saveAfter = start.Add(450 * time.Millisecond)
	check(start, false, 250*time.Millisecond)
	first := start.Add(250 * time.Millisecond)
	a.tick(first)
	if !a.settingsPending {
		t.Fatal("autosave ran before its deadline")
	}
	check(first, false, 200*time.Millisecond)
	deadline := a.saveAfter
	a.tick(deadline)
	got, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || got.Interval != 15*time.Minute || a.settingsPending {
		t.Fatalf("autosave at deadline: interval=%v, confirmed=%t, pending=%t, err=%v", got.Interval, confirmed, a.settingsPending, err)
	}
	check(deadline, false, time.Second)
}

func TestStatusChangedUsesDisplayedSecond(t *testing.T) {
	before := breaks.State{Phase: breaks.Waiting, Kind: breaks.Short, Remaining: 4900 * time.Millisecond}
	after := before
	after.Remaining -= 250 * time.Millisecond
	if statusChanged(before, after) {
		t.Fatal("same displayed second marked dirty")
	}
	after.Remaining = 4 * time.Second
	if !statusChanged(before, after) {
		t.Fatal("new displayed second not marked dirty")
	}
	after = before
	after.Kind = breaks.Long
	if !statusChanged(before, after) {
		t.Fatal("break kind change not marked dirty")
	}
	after = before
	after.Phase = breaks.Paused
	if !statusChanged(before, after) {
		t.Fatal("phase change not marked dirty")
	}
}
