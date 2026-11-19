package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

func TestResolveLanguage(t *testing.T) {
	for _, tc := range []struct {
		choice    i18n.Language
		preferred []string
		want      i18n.Language
	}{
		{i18n.Spanish, []string{"fr-CA"}, i18n.Spanish},
		{i18n.System, []string{"de-DE", "fr-CA", "en-US"}, i18n.French},
		{i18n.System, []string{"zh-Hant-TW", "en-US"}, i18n.Chinese},
		{i18n.System, []string{"ar-EG"}, i18n.Arabic},
		{i18n.System, []string{"de-DE"}, i18n.English},
	} {
		if got := resolveLanguage(tc.choice, tc.preferred); got != tc.want {
			t.Errorf("resolveLanguage(%q, %v) = %q, want %q", tc.choice, tc.preferred, got, tc.want)
		}
	}
}

func newTestDesktop(t *testing.T) *desktop {
	t.Helper()
	settings := prefs.Defaults()
	now := time.Now()
	scheduler := breaks.New(settings.Settings, now)
	return &desktop{language: i18n.English, prefs: settings, draft: newDraft(settings),
		path: filepath.Join(t.TempDir(), "settings.json"), scheduler: scheduler, state: scheduler.State(now)}
}

func TestLanguageSelectionAppliesAndPersists(t *testing.T) {
	a := newTestDesktop(t)
	tester := ui.NewTester(a.settingsView, 860, 660)
	if err := tester.Click(i18n.English.Text(i18n.SystemLanguage)); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("العربية"); err != nil {
		t.Fatal(err)
	}
	if a.draft.language != i18n.Arabic || a.language != i18n.Arabic {
		t.Fatalf("draft language %q, displayed language %q", a.draft.language, a.language)
	}
	settings, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || settings.Language != i18n.Arabic || settings.Settings != prefs.Defaults().Settings {
		t.Fatalf("loaded %+v, confirmed %v, error %v", settings, confirmed, err)
	}
}

func TestSettingsFillsTiledWindow(t *testing.T) {
	tr := i18n.English.Text
	for _, page := range []settingsPage{generalPage, shortcutsPage, schedulePage} {
		a := newTestDesktop(t)
		a.settingsPage = page
		for _, height := range []int{460, 1040} {
			tester := ui.NewTester(a.settingsView, 800, height)
			for _, check := range []struct {
				label  string
				bottom bool
			}{
				{tr(i18n.Appearance), false},
				{tr(i18n.StartBreakNow), true},
				{tr(i18n.AutoSaveHelp), true},
			} {
				rect, found := tester.Find(check.label)
				if !found || (!check.bottom && rect.Y >= 180) || (check.bottom && rect.Y+rect.H < float32(height-24)) {
					t.Errorf("page %d, height %d: %q found %v at %+v", page, height, check.label, found, rect)
				}
			}
		}
	}
}

func TestScheduleAutoSaveRejectsInvalidDraft(t *testing.T) {
	a := newTestDesktop(t)
	tester := ui.NewTester(a.settingsView, 860, 660)
	tr := i18n.English.Text
	if err := tester.Click(tr(i18n.Schedule)); err != nil {
		t.Fatal(err)
	}
	input := tr(i18n.SecondsLabel, tr(i18n.WorkInterval))
	if err := tester.Click(input); err != nil {
		t.Fatal(err)
	}
	for range len(a.draft.interval) {
		tester.Key(0, ui.KeyBackspace)
	}
	a.flushSettings()
	if !strings.Contains(a.settingsInfo.text(a.language), tr(i18n.WorkInterval)) || a.prefs.Interval != 10*time.Minute {
		t.Fatalf("invalid draft changed preferences or lacked an error: %v, %+v", a.prefs.Interval, a.settingsInfo)
	}
	tester.Type("1500")
	a.flushSettings()
	settings, confirmed, err := prefs.Load(a.path)
	if err != nil || !confirmed || settings.Interval != 25*time.Minute || a.state.Remaining != 25*time.Minute {
		t.Fatalf("loaded %+v, confirmed %v, error %v, state %+v", settings, confirmed, err, a.state)
	}
}

func TestDiscreteSettingsSaveWithInvalidScheduleDraft(t *testing.T) {
	a := newTestDesktop(t)
	a.settingsPage = schedulePage
	tester := ui.NewTester(a.settingsView, 860, 660)
	tr := a.language.Text
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(tr(i18n.SecondsLabel, tr(i18n.WorkInterval)))
	for range len(a.draft.interval) {
		tester.Key(0, ui.KeyBackspace)
	}
	check := func(want prefs.Settings) {
		t.Helper()
		disk, confirmed, err := prefs.Load(a.path)
		if err != nil || !confirmed || disk != want || a.prefs != want {
			t.Fatalf("preferences: disk %+v, applied %+v, want %+v, confirmed %v, err %v", disk, a.prefs, want, confirmed, err)
		}
		if a.draft.interval != "" || a.settingsInfo.field != i18n.WorkInterval || a.settingsInfo.key != i18n.WholeNumberRange {
			t.Fatalf("invalid draft or error lost: interval %q, message %+v", a.draft.interval, a.settingsInfo)
		}
	}
	click(tr(i18n.ShortStrict))
	want := prefs.Defaults()
	want.Short.Strict = true
	check(want)
	click(tr(i18n.Appearance))
	click(tr(i18n.ThemeLight))
	want.Theme = prefs.ThemeLight
	check(want)
	click(tr(i18n.General))
	click(tr(i18n.SystemLanguage))
	click("العربية")
	want.Language = i18n.Arabic
	check(want)
	if a.language != i18n.Arabic {
		t.Fatalf("language was not applied: %v", a.language)
	}

	tr = a.language.Text
	click(tr(i18n.Schedule))
	click(tr(i18n.SecondsLabel, tr(i18n.WorkInterval)))
	tester.Type("1500")
	a.flushSettings()
	disk, _, err := prefs.Load(a.path)
	want.Interval = 25 * time.Minute
	if err != nil || disk != want || a.prefs != want || a.state.Remaining != want.Interval {
		t.Fatalf("corrected schedule lost discrete changes: disk %+v, applied %+v, state %+v, err %v", disk, a.prefs, a.state, err)
	}
}

func TestDiscreteSaveLeavesValidNumericEditPending(t *testing.T) {
	a := newTestDesktop(t)
	a.draft.interval = "1500"
	a.scheduleSettingsSave()
	deadline := a.saveAfter
	a.draft.theme = prefs.ThemeLight
	a.saveDiscreteSettings()
	disk, _, err := prefs.Load(a.path)
	if err != nil || disk.Theme != prefs.ThemeLight || disk.Interval != prefs.Defaults().Interval || !a.settingsPending || a.saveAfter != deadline {
		t.Fatalf("discrete save consumed numeric edit: disk %+v, pending %t, deadline %v, err %v", disk, a.settingsPending, a.saveAfter, err)
	}
	a.tick(deadline)
	disk, _, err = prefs.Load(a.path)
	if err != nil || disk.Theme != prefs.ThemeLight || disk.Interval != 25*time.Minute || a.settingsPending {
		t.Fatalf("numeric autosave lost settings: disk %+v, pending %t, err %v", disk, a.settingsPending, err)
	}
}

func TestFailedDiscreteSaveRetriesWithInvalidScheduleDraft(t *testing.T) {
	a := newTestDesktop(t)
	path := a.path
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	a.path = filepath.Join(blocker, "settings.json")
	a.draft.interval = ""
	a.scheduleSettingsSave()
	a.draft.theme = prefs.ThemeLight
	a.saveDiscreteSettings()
	if !a.discretePending || a.settingsInfo.key != i18n.SaveFailed || a.prefs.Theme != prefs.ThemeDark {
		t.Fatalf("failed save changed applied preferences or lost the pending edit: %+v, %+v", a.prefs, a.settingsInfo)
	}
	a.path = path
	a.flushSettings()
	disk, _, err := prefs.Load(path)
	if err != nil || disk.Theme != prefs.ThemeLight || disk.Interval != prefs.Defaults().Interval || a.settingsPending || a.discretePending {
		t.Fatalf("retry did not save just the valid edit: disk %+v, pending %t/%t, err %v", disk, a.settingsPending, a.discretePending, err)
	}
	if a.draft.interval != "" || a.settingsInfo.field != i18n.WorkInterval || a.settingsInfo.key != i18n.WholeNumberRange {
		t.Fatalf("retry lost numeric draft or error: interval %q, message %+v", a.draft.interval, a.settingsInfo)
	}
}

func TestDraftParsesArabicDigitsAndLocalizesErrors(t *testing.T) {
	draft := newDraft(prefs.Defaults())
	draft.interval = " ٦٠٠ "
	settings, message := draft.parse()
	if message.text(i18n.Arabic) != "" || settings.Interval != 10*time.Minute {
		t.Fatalf("Arabic digits: settings %+v, message %+v", settings, message)
	}
	draft.short.notice = "٦٠٠"
	_, message = draft.parse()
	want := i18n.Arabic.Text(i18n.NoticeBelowInterval, i18n.Arabic.Text(i18n.ShortNotice))
	if got := message.text(i18n.Arabic); got != want {
		t.Errorf("localized validation: got %q, want %q", got, want)
	}
}
