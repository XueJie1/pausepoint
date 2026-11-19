package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

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

func TestLanguageSelectionUpdatesDraft(t *testing.T) {
	a := &desktop{language: i18n.English, draft: newDraft(prefs.Defaults())}
	tester := ui.NewTester(a.settingsView, 610, 590)
	if err := tester.Click(i18n.English.Text(i18n.SystemLanguage)); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("العربية"); err != nil {
		t.Fatal(err)
	}
	if a.draft.language != i18n.Arabic || a.language != i18n.English {
		t.Fatalf("draft language %q, displayed language %q", a.draft.language, a.language)
	}
	settings, message := a.draft.parse()
	if message.text(a.language) != "" || settings.Language != i18n.Arabic || settings.Settings != prefs.Defaults().Settings {
		t.Fatalf("parsed draft %+v, message %+v", settings, message)
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
