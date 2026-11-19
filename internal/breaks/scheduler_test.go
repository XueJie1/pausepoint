package breaks

import (
	"testing"
	"time"
)

var origin = time.Date(2026, time.January, 1, 9, 0, 0, 0, time.UTC)

func TestDefaultCycleAndNotices(t *testing.T) {
	s := New(Defaults(), origin)
	now := origin
	for _, want := range []struct {
		kind         Kind
		lead, length time.Duration
	}{
		{Short, 10 * time.Second, 20 * time.Second},
		{Short, 10 * time.Second, 20 * time.Second},
		{Long, 30 * time.Second, 5 * time.Minute},
		{Short, 10 * time.Second, 20 * time.Second},
	} {
		if got := s.State(now); got.Kind != want.kind || got.Phase != Waiting || got.Remaining != 10*time.Minute {
			t.Fatalf("unexpected scheduled break: %+v", got)
		}
		start := now.Add(10 * time.Minute)
		notice := start.Add(-want.lead)
		if s.Advance(notice.Add(-time.Nanosecond)) || !s.Advance(notice) || s.Advance(notice) {
			t.Fatal("expected exactly one notice at the lead time")
		}
		if s.Advance(start) {
			t.Fatal("break start should not also send a notice")
		}
		if got := s.State(start); got.Phase != Active || got.Remaining != want.length {
			t.Fatalf("unexpected active break: %+v", got)
		}
		now = start.Add(want.length)
		s.Advance(now)
	}
}

func TestPostponeAndSkip(t *testing.T) {
	s := New(Defaults(), origin)
	s.Start(origin)
	if s.Postpone(origin.Add(6 * time.Second)) {
		t.Fatal("postponement allowed after the first 30%")
	}
	// Use a fresh scheduler to keep time moving forward in both cases.
	s = New(Defaults(), origin)
	now := origin.Add(time.Second)
	s.Start(now)
	if !s.Postpone(now) {
		t.Fatal("could not postpone at the start of a break")
	}
	if got := s.State(now); got.Kind != Short || got.Phase != Waiting || got.Remaining != 2*time.Minute {
		t.Fatalf("unexpected postponement: %+v", got)
	}
	if !s.Advance(now.Add(110 * time.Second)) {
		t.Fatal("postponed break should issue a new notice")
	}
	now = now.Add(2 * time.Minute)
	s.Advance(now)
	if s.Postpone(now) || !s.Skip(now) {
		t.Fatal("could postpone twice, or could not skip a non-strict break")
	}
	s.Start(now)
	s.Skip(now)
	if got := s.State(now); got.Kind != Long || got.Phase != Waiting {
		t.Fatalf("skip should count in the cycle: %+v", got)
	}
}

func TestPauseAndStrictMode(t *testing.T) {
	settings := Defaults()
	settings.Short.Strict = true
	s := New(settings, origin)
	s.Start(origin)
	s.Reconfigure(Defaults(), origin)
	if s.Pause() || s.Skip(origin) || s.Postpone(origin) {
		t.Fatal("a strict break could be interrupted")
	}
	s.Advance(origin.Add(20 * time.Second))
	if !s.Pause() {
		t.Fatal("could not pause after the break ended")
	}
	now := origin.Add(time.Hour)
	s.Suspend()
	s.Wake(now)
	if s.Advance(now) || s.State(now).Phase != Paused || s.Start(now) {
		t.Fatal("sleep or time passing ended a manual pause")
	}
	if !s.Resume(now) || s.State(now).Remaining != settings.Interval {
		t.Fatal("resume should start a full work interval")
	}
}

func TestWakeDiscardsActiveAndMissedBreaks(t *testing.T) {
	s := New(Defaults(), origin)
	s.Start(origin)
	s.Suspend()
	now := origin.Add(12 * time.Hour)
	if got := s.State(now); got.Phase != Sleeping || s.Advance(now) {
		t.Fatalf("unexpected state while suspended: %+v", got)
	}
	s.Wake(now)
	if got := s.State(now); got.Phase != Waiting || got.Kind != Short || got.Remaining != 10*time.Minute || s.Advance(now) {
		t.Fatalf("unexpected state after wake: %+v", got)
	}
	late := now.Add(11 * time.Minute)
	if s.Advance(late) || s.State(late).Remaining != 20*time.Second {
		t.Fatal("a late tick should start the break without a stale notice")
	}
}

func TestReconfigureDuringPausedSuspend(t *testing.T) {
	s := New(Defaults(), origin)
	s.Pause()
	s.Suspend()
	settings := Defaults()
	settings.Interval = 25 * time.Minute
	s.Reconfigure(settings, origin.Add(time.Second))

	wake := origin.Add(time.Hour)
	s.Wake(wake)
	if got := s.State(wake); got.Phase != Paused {
		t.Fatalf("wake ended manual pause: %+v", got)
	}
	if !s.Resume(wake) {
		t.Fatal("could not resume after wake")
	}
	if got := s.State(wake); got.Phase != Waiting || got.Remaining != settings.Interval {
		t.Fatalf("resume did not use new settings: %+v", got)
	}
}

func TestReconfigureDuringBreakAppliesToLaterBreaks(t *testing.T) {
	s := New(Defaults(), origin)
	s.Start(origin)
	now := origin.Add(time.Second)
	before := s.State(now)
	settings := Defaults()
	settings.Interval = 25 * time.Minute
	settings.Short.Duration = 40 * time.Second
	settings.Short.Postpone = 0
	settings.Short.Strict = true
	s.Reconfigure(settings, now)
	if got := s.State(now); got != before {
		t.Fatalf("reconfigure changed active break: before %+v, after %+v", before, got)
	}

	end := origin.Add(Defaults().Short.Duration)
	s.Advance(end)
	if got := s.State(end); got.Phase != Waiting || got.Remaining != settings.Interval {
		t.Fatalf("work interval did not use new settings: %+v", got)
	}
	s.Start(end)
	if got := s.State(end); got.Phase != Active || got.Duration != settings.Short.Duration || got.CanSkip || got.CanPostpone {
		t.Fatalf("next break did not use new rule: %+v", got)
	}
}

func TestPostponementShorterThanNotice(t *testing.T) {
	settings := Defaults()
	settings.Short.Postpone = 5 * time.Second
	s := New(settings, origin)
	s.Start(origin)
	s.Postpone(origin)
	if s.Advance(origin.Add(time.Second)) || s.Advance(origin.Add(5*time.Second)) || s.State(origin.Add(5*time.Second)).Phase != Active {
		t.Fatal("short postponement should restart the break without a notice")
	}
}
