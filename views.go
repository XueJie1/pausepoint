package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

type settingsDraft struct {
	language        i18n.Language
	theme           prefs.Theme
	previewShortcut prefs.Shortcut
	interval        string
	short, long     ruleDraft
}

type ruleDraft struct {
	duration, notice, postpone string
	strict                     bool
}

type formMessage struct {
	key   i18n.Key
	field i18n.Key
}

func (m formMessage) text(language i18n.Language) string {
	if m.key == 0 {
		return ""
	}
	if m.field != 0 {
		return language.Text(m.key, language.Text(m.field))
	}
	return language.Text(m.key)
}

var settingLabels = [...]i18n.Key{
	breaks.Interval: i18n.WorkInterval, breaks.ShortDuration: i18n.ShortDuration,
	breaks.ShortNotice: i18n.ShortNotice, breaks.ShortPostpone: i18n.ShortPostpone,
	breaks.LongDuration: i18n.LongDuration, breaks.LongNotice: i18n.LongNotice,
	breaks.LongPostpone: i18n.LongPostpone,
}

func newDraft(settings prefs.Settings) settingsDraft {
	seconds := func(d time.Duration) string { return strconv.FormatInt(int64(d/time.Second), 10) }
	rule := func(r breaks.Rule) ruleDraft {
		return ruleDraft{seconds(r.Duration), seconds(r.Notice), seconds(r.Postpone), r.Strict}
	}
	return settingsDraft{
		language: settings.Language, theme: settings.Theme, previewShortcut: settings.PreviewShortcut,
		interval: seconds(settings.Interval), short: rule(settings.Short), long: rule(settings.Long),
	}
}

func (d settingsDraft) withDiscrete(settings prefs.Settings) prefs.Settings {
	settings.Language, settings.Theme, settings.PreviewShortcut = d.language, d.theme, d.previewShortcut
	settings.Short.Strict, settings.Long.Strict = d.short.strict, d.long.strict
	return settings
}

func (d settingsDraft) parse() (prefs.Settings, formMessage) {
	return d.withNumbers(d.withDiscrete(prefs.Settings{}))
}

func (d settingsDraft) withNumbers(out prefs.Settings) (prefs.Settings, formMessage) {
	fields := [...]struct {
		value string
		out   *time.Duration
	}{
		breaks.Interval:      {d.interval, &out.Interval},
		breaks.ShortDuration: {d.short.duration, &out.Short.Duration},
		breaks.ShortNotice:   {d.short.notice, &out.Short.Notice},
		breaks.ShortPostpone: {d.short.postpone, &out.Short.Postpone},
		breaks.LongDuration:  {d.long.duration, &out.Long.Duration},
		breaks.LongNotice:    {d.long.notice, &out.Long.Notice},
		breaks.LongPostpone:  {d.long.postpone, &out.Long.Postpone},
	}
	for field, input := range fields {
		digits := strings.Map(asciiDigit, strings.TrimSpace(input.value))
		value, err := strconv.ParseUint(digits, 10, 32)
		if err != nil || value > uint64(breaks.MaxDuration/time.Second) {
			return out, formMessage{i18n.WholeNumberRange, settingLabels[field]}
		}
		*input.out = time.Duration(value) * time.Second
	}
	if err := out.Settings.Validate(); err != nil {
		validation, ok := err.(breaks.ValidationError)
		if !ok {
			return out, formMessage{key: i18n.SaveFailed}
		}
		key := i18n.PositiveSeconds
		switch validation.Field {
		case breaks.ShortNotice, breaks.LongNotice:
			key = i18n.NoticeBelowInterval
		case breaks.ShortPostpone, breaks.LongPostpone:
			key = i18n.NonNegativeSeconds
		}
		return out, formMessage{key, settingLabels[validation.Field]}
	}
	return out, formMessage{}
}

func asciiDigit(r rune) rune {
	for _, zero := range []rune{'٠', '۰', '０'} {
		if r >= zero && r <= zero+9 {
			return '0' + r - zero
		}
	}
	return r
}

func resolveLanguage(choice i18n.Language, preferred []string) i18n.Language {
	if choice != i18n.System && choice.Valid() {
		return choice
	}
	for _, tag := range preferred {
		base, _, _ := strings.Cut(tag, "-")
		if language := i18n.Language(strings.ToLower(base)); language != i18n.System && language.Valid() {
			return language
		}
	}
	return i18n.English
}

func viewDirection(language i18n.Language) ui.Direction {
	if language.RightToLeft() {
		return ui.RightToLeft
	}
	return ui.LeftToRight
}

func countdown(d time.Duration) string {
	seconds := int((max(0, d) + time.Second - 1) / time.Second)
	if seconds >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func breakLabel(kind breaks.Kind) i18n.Key {
	if kind == breaks.Long {
		return i18n.LongBreak
	}
	return i18n.ShortBreak
}

func statusText(language i18n.Language, s breaks.State) string {
	switch s.Phase {
	case breaks.Paused:
		return language.Text(i18n.Paused)
	case breaks.Sleeping:
		return language.Text(i18n.Sleeping)
	}
	key := i18n.WaitingStatus
	if s.Phase == breaks.Active {
		key = i18n.ActiveStatus
	}
	return language.Text(key, language.Text(breakLabel(s.Kind)), countdown(s.Remaining))
}

func pauseLabel(language i18n.Language, s breaks.State) string {
	if s.Phase == breaks.Paused {
		return language.Text(i18n.ResumeReminders)
	}
	return language.Text(i18n.PauseReminders)
}

func canTogglePause(s breaks.State) bool {
	return s.Phase != breaks.Sleeping && (s.Phase != breaks.Active || s.CanSkip)
}
