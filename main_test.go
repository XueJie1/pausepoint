package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

func TestQuitSavesPendingEditWhileResting(t *testing.T) {
	a := newTestDesktop(t)
	a.settingsPage = schedulePage
	tester := ui.NewTester(a.settingsView, 860, 660)
	tr := a.language.Text
	if err := tester.Click(tr(i18n.SecondsLabel, tr(i18n.WorkInterval))); err != nil {
		t.Fatal(err)
	}
	for range len(a.draft.interval) {
		tester.Key(0, ui.KeyBackspace)
	}
	tester.Type("900")
	if !a.settingsPending || a.saveAfter.IsZero() {
		t.Fatal("editing should schedule an autosave")
	}
	start := a.saveAfter.Add(-time.Millisecond)
	a.scheduler = breaks.New(a.prefs.Settings, start.Add(-a.prefs.Interval))
	a.state = a.scheduler.State(start)

	a.tick(start)
	if a.state.Phase != breaks.Active || !a.settingsPending || a.prefs.Interval != 10*time.Minute {
		t.Fatalf("expected active rest with pending edit, got state %+v, pending %t, prefs %+v", a.state, a.settingsPending, a.prefs)
	}
	a.beforeQuit(&mygo.QuitEvent{})
	got, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || got.Interval != 15*time.Minute || !a.quitting || a.settingsPending {
		t.Fatalf("quit did not save pending edit: got %+v, confirmed %t, quitting %t, pending %t, err %v", got, confirmed, a.quitting, a.settingsPending, err)
	}
	if a.state.Phase != breaks.Active || a.state.Duration != a.prefs.Short.Duration {
		t.Fatalf("saving interrupted the active rest: %+v", a.state)
	}
}

func TestQuitRetainsPendingEditOnWriteFailure(t *testing.T) {
	a := newTestDesktop(t)
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	path := a.path
	a.path = filepath.Join(blocker, "settings.json")
	a.draft.interval = "900"
	a.settingsPending = true
	a.saveAfter = time.Now()

	a.beforeQuit(&mygo.QuitEvent{})
	if a.quitting || !a.settingsPending || a.settingsInfo.key == 0 || a.prefs.Interval != 10*time.Minute {
		t.Fatalf("failed write should block quit and retain the draft: quitting %t, pending %t, info %+v, interval %v", a.quitting, a.settingsPending, a.settingsInfo, a.prefs.Interval)
	}
	a.path = path
	a.beforeQuit(&mygo.QuitEvent{})
	got, _, err := prefs.Load(path)
	if err != nil || !a.quitting || a.settingsPending || got.Interval != 15*time.Minute {
		t.Fatalf("quit retry did not save pending edit: got %+v, quitting %t, pending %t, err %v", got, a.quitting, a.settingsPending, err)
	}
}

func TestPausedSuspendedEditAppliesAfterWake(t *testing.T) {
	a := newTestDesktop(t)
	now := time.Now()
	a.scheduler.Pause()
	a.scheduler.Suspend()
	a.refresh(now)
	a.draft.interval = "900"
	a.settingsPending = true
	a.saveAfter = now
	a.tick(now)
	got, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || got.Interval != 15*time.Minute || a.prefs.Interval != got.Interval {
		t.Fatalf("paused edit was not saved: disk interval %v, runtime interval %v, confirmed %t, err %v", got.Interval, a.prefs.Interval, confirmed, err)
	}
	wokeAt := now.Add(time.Hour)
	a.wake(wokeAt)
	if a.state.Phase != breaks.Paused || !a.scheduler.Resume(wokeAt) {
		t.Fatalf("wake did not preserve manual pause: %+v", a.state)
	}
	if got := a.scheduler.State(wokeAt).Remaining; got != 15*time.Minute {
		t.Fatalf("resumed with interval %v, want 15m", got)
	}
}

func TestTickTimeJumpsPreserveRestAndCycle(t *testing.T) {
	a := newTestDesktop(t)
	start := time.Unix(1000, 0)
	a.scheduler = breaks.New(a.prefs.Settings, start)
	a.state = a.scheduler.State(start)
	for range 2 {
		a.scheduler.Start(start)
		a.scheduler.Skip(start)
	}
	a.scheduler.Start(start)
	a.refresh(start)

	late := start.Add(30 * time.Second)
	a.tick(late)
	if a.state.Phase != breaks.Active || a.state.Kind != breaks.Long || a.state.Remaining != 4*time.Minute+30*time.Second {
		t.Fatalf("late tick reset the active long break: %+v", a.state)
	}
	back := late.Add(-2 * time.Second)
	a.tick(back)
	if a.state.Phase != breaks.Active || a.state.Kind != breaks.Long || a.state.Remaining != 4*time.Minute+32*time.Second {
		t.Fatalf("backward timestamp reset the active long break: %+v", a.state)
	}
	a.tick(start.Add(20 * time.Minute))
	if a.state.Phase != breaks.Waiting || a.state.Kind != breaks.Short || a.state.Remaining != a.prefs.Interval {
		t.Fatalf("missed ticks should finish one break and schedule the next: %+v", a.state)
	}
}
