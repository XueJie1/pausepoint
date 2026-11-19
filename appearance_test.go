package main

import (
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

func TestThemeSelectionAutoSaves(t *testing.T) {
	settings := prefs.Defaults()
	settings.Language = i18n.English
	now := time.Now()
	a := &desktop{
		prefs: settings, draft: newDraft(settings), settingsPage: appearancePage,
		language: i18n.English, path: filepath.Join(t.TempDir(), "settings.json"),
		scheduler: breaks.New(settings.Settings, now),
	}
	tester := ui.NewTester(a.settingsView, 780, 620)
	if err := tester.Click(a.language.Text(i18n.ThemeLight)); err != nil {
		t.Fatal(err)
	}
	got, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || got.Theme != prefs.ThemeLight || a.prefs.Theme != prefs.ThemeLight {
		t.Fatalf("theme persisted: got %+v, confirmed %t, err %v, active theme %q", got, confirmed, err, a.prefs.Theme)
	}
	if got.Settings != settings.Settings {
		t.Fatalf("changing theme modified schedule: got %+v, want %+v", got.Settings, settings.Settings)
	}
}

func TestSettingsAppearanceRendersSelectedTheme(t *testing.T) {
	light := color.RGBA{R: 0xF6, G: 0xF6, B: 0xF7, A: 0xFF}
	dark := color.RGBA{R: 0x0D, G: 0x0D, B: 0x0E, A: 0xFF}
	for _, tc := range []struct {
		name        string
		choice      prefs.Theme
		wantOnLight color.RGBA
		wantOnDark  color.RGBA
	}{
		{"light override", prefs.ThemeLight, light, light},
		{"dark override", prefs.ThemeDark, dark, dark},
		{"system switch", prefs.ThemeSystem, light, dark},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestDesktop(t)
			a.settingsPage, a.prefs.Theme, a.draft.theme = appearancePage, tc.choice, tc.choice
			tester := ui.NewTester(a.settingsView, 780, 620)
			for _, system := range []struct {
				dark bool
				want color.RGBA
			}{{false, tc.wantOnLight}, {true, tc.wantOnDark}, {false, tc.wantOnLight}} {
				tester.SetDark(system.dark)
				if got := tester.Image().RGBAAt(750, 15); got != system.want {
					t.Fatalf("settings background with system dark %t = %v, want %v", system.dark, got, system.want)
				}
			}
		})
	}
}

func TestBreakOverlayRendersThemeAndActions(t *testing.T) {
	a := newTestDesktop(t)
	a.prefs.Theme = prefs.ThemeSystem
	a.state = breaks.State{Phase: breaks.Active, Kind: breaks.Short, Duration: 20 * time.Second,
		Remaining: 10 * time.Second, CanSkip: true, CanPostpone: true}
	tester := ui.NewTester(a.breakView, breakWindowWidth, breakWindowHeight)
	tester.SetDark(false)
	if got, want := tester.Image().RGBAAt(5, 5), (color.RGBA{R: 0xF6, G: 0xF6, B: 0xF7, A: 0xFF}); got != want {
		t.Fatalf("light break background = %v, want %v", got, want)
	}
	tester.SetDark(true)
	if got, want := tester.Image().RGBAAt(5, 5), (color.RGBA{R: 0x0D, G: 0x0D, B: 0x0E, A: 0xFF}); got != want {
		t.Fatalf("dark break background = %v, want %v", got, want)
	}
	for _, key := range []i18n.Key{i18n.Postpone, i18n.SkipBreak} {
		if _, ok := tester.Find(a.language.Text(key)); !ok {
			t.Fatalf("missing break action %q", a.language.Text(key))
		}
	}
}

func TestPreviewDismissesWithoutChangingTheSchedule(t *testing.T) {
	settings := prefs.Defaults()
	a := &desktop{prefs: settings, language: i18n.English}
	tester := ui.NewTester(a.previewView, breakWindowWidth, breakWindowHeight)
	now := tester.Now()
	a.scheduler = breaks.New(settings.Settings, now)
	a.state = a.scheduler.State(now)
	a.previewUntil = now.Add(settings.Short.Duration)
	tester.Frame()
	if _, ok := tester.Find(a.language.Text(i18n.ShortBreak)); !ok {
		t.Fatal("preview did not render the short break")
	}
	tester.Key(0, ui.KeyEscape)
	if !a.previewUntil.IsZero() || a.state != a.scheduler.State(now) || a.state.Phase != breaks.Waiting {
		t.Fatalf("preview still active or changed schedule: %v, %+v", a.previewUntil, a.state)
	}
}

func TestRecordingPreviewShortcutAutoSavesAndResets(t *testing.T) {
	settings := prefs.Defaults()
	settings.Language = i18n.English
	now := time.Now()
	a := &desktop{
		prefs: settings, draft: newDraft(settings), settingsPage: shortcutsPage,
		language: i18n.English, path: filepath.Join(t.TempDir(), "settings.json"),
		scheduler: breaks.New(settings.Settings, now),
	}
	tester := ui.NewTester(a.settingsView, 780, 620)
	tr := a.language.Text
	if err := tester.Click(tr(i18n.RecordShortcut)); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyX)
	if !a.previewRecording || a.settingsInfo.key != i18n.ShortcutInvalid {
		t.Fatalf("unmodified key: recording %t, info %#v", a.previewRecording, a.settingsInfo)
	}
	tester.Key(ui.Cmd|ui.Shift, ui.KeyX)
	got, _, err := prefs.Load(a.path)
	want := prefs.Shortcut{Key: "X", Shift: true}
	if err != nil || a.previewRecording || got.PreviewShortcut != want || a.prefs.PreviewShortcut != want {
		t.Fatalf("recorded shortcut: got %+v, active %+v, recording %t, err %v", got.PreviewShortcut, a.prefs.PreviewShortcut, a.previewRecording, err)
	}
	if err := tester.Click(tr(i18n.ResetShortcut)); err != nil {
		t.Fatal(err)
	}
	got, _, err = prefs.Load(a.path)
	if err != nil || got.PreviewShortcut != (prefs.Shortcut{}) || a.prefs.PreviewShortcut != (prefs.Shortcut{}) {
		t.Fatalf("reset shortcut: got %+v, active %+v, err %v", got.PreviewShortcut, a.prefs.PreviewShortcut, err)
	}
}
