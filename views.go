package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

type settingsDraft struct {
	language    i18n.Language
	interval    string
	short, long ruleDraft
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
	return settingsDraft{settings.Language, seconds(settings.Interval), rule(settings.Short), rule(settings.Long)}
}

func (d settingsDraft) parse() (prefs.Settings, formMessage) {
	out := prefs.Settings{Language: d.language}
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
	out.Short.Strict, out.Long.Strict = d.short.strict, d.long.strict
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

func (a *desktop) saveSettings() {
	if a.state.Phase == breaks.Active || a.state.Phase == breaks.Sleeping {
		return
	}
	settings, message := a.draft.parse()
	if message.key != 0 {
		a.settingsInfo = message
		return
	}
	if a.path == "" {
		a.settingsInfo = formMessage{key: i18n.ConfigUnavailable}
		return
	}
	if err := prefs.Save(a.path, settings); err != nil {
		log.Print("save settings: ", err)
		a.settingsInfo = formMessage{key: i18n.SaveFailed}
		return
	}
	now := time.Now()
	if settings.Settings != a.prefs.Settings {
		a.scheduler.Reconfigure(settings.Settings, now)
		a.clearNotice()
	}
	previousLanguage := a.language
	a.prefs, a.draft = settings, newDraft(settings)
	firstConfirmation := !a.confirmed
	a.confirmed = true
	a.settingsInfo = formMessage{key: i18n.Saved}
	a.updateLanguage()
	a.refresh(now)
	if previousLanguage != a.language && a.notification != nil {
		a.showNotice()
	}
	if firstConfirmation && a.tray != nil {
		a.settings.Hide()
	}
}

func (a *desktop) settingsView(c *ui.Context) {
	tr := a.language.Text
	theme := c.Theme()
	ui.Scroll(c).Fill().Direction(viewDirection(a.language)).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(680).AlignSelf(ui.Center).Padding(26).Gap(18).Children(func() {
			ui.Column(c).Gap(7).Children(func() {
				ui.Text(c, tr(i18n.BreakReminders)).FontSize(27).Bold()
				ui.Text(c, statusText(a.language, a.state)).FontSize(17).TextColor(theme.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, pauseLabel(a.language, a.state)).Disabled(!canTogglePause(a.state)).Clicked() {
						a.togglePause()
					}
					if ui.Button(c, tr(i18n.StartBreakNow)).Disabled(a.state.Phase != breaks.Waiting).Clicked() {
						a.startBreak()
					}
				})
			})
			if a.notifyFailed {
				ui.Text(c, tr(i18n.NotificationsUnavailable)).TextColor(theme.TextMuted).Wrap()
			}
			a.languageField(c)
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, tr(i18n.Schedule)).FontSize(18).Bold()
				interval := tr(i18n.WorkInterval)
				settingInput(c, interval, tr(i18n.SecondsLabel, interval), &a.draft.interval)
				ui.Text(c, tr(i18n.ScheduleHelp)).TextColor(theme.TextMuted).Wrap()
			})
			ui.Row(c).FillWidth().Gap(20).Children(func() {
				ruleFields(c, a.language, breaks.Short, &a.draft.short)
				ruleFields(c, a.language, breaks.Long, &a.draft.long)
			})
			ui.Text(c, tr(i18n.RuleHelp)).TextColor(theme.TextMuted).Wrap()
			ui.Row(c).AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
				saveDisabled := a.state.Phase == breaks.Active || a.state.Phase == breaks.Sleeping
				saveLabel := i18n.SaveSettings
				if !a.confirmed {
					saveLabel = i18n.ConfirmSettings
				}
				if ui.PrimaryButton(c, tr(saveLabel)).Disabled(saveDisabled).Clicked() {
					a.saveSettings()
					c.Invalidate()
				}
				if ui.Button(c, tr(i18n.RestoreDefaults)).Clicked() {
					a.draft = newDraft(prefs.Defaults())
					a.settingsInfo = formMessage{key: i18n.DefaultsPending}
					c.Invalidate()
				}
				ui.Spacer(c).Grow(1)
				if ui.Button(c, tr(i18n.Close)).Clicked() {
					a.settings.Hide()
				}
				if ui.Button(c, tr(i18n.Quit)).Clicked() {
					mygo.App.Quit()
				}
			})
			if info := a.settingsInfo.text(a.language); info != "" {
				ui.Text(c, info).TextColor(theme.TextMuted).Wrap()
			}
		})
	})
}

func (a *desktop) languageField(c *ui.Context) {
	label := a.language.Text(i18n.LanguageLabel)
	system := a.language.Text(i18n.SystemLanguage)
	selected := system
	choices := i18n.Languages()
	options := make([]string, 1, len(choices)+1)
	options[0] = system
	for _, choice := range choices {
		options = append(options, choice.Name)
		if choice.Code == a.draft.language {
			selected = choice.Name
		}
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(112).Wrap()
		ui.Select(c, &selected, options).Label(label).OnChange(func() {
			if selected == system {
				a.draft.language = i18n.System
				return
			}
			for _, choice := range choices {
				if choice.Name == selected {
					a.draft.language = choice.Code
					return
				}
			}
		})
	})
}

func ruleFields(c *ui.Context, language i18n.Language, kind breaks.Kind, draft *ruleDraft) {
	tr := language.Text
	kindLabel, duration, notice, postpone, strict := i18n.ShortBreak, i18n.ShortDuration, i18n.ShortNotice, i18n.ShortPostpone, i18n.ShortStrict
	if kind == breaks.Long {
		kindLabel, duration, notice, postpone, strict = i18n.LongBreak, i18n.LongDuration, i18n.LongNotice, i18n.LongPostpone, i18n.LongStrict
	}
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(6).Children(func() {
		ui.Text(c, tr(kindLabel)).FontSize(16).Bold()
		settingInput(c, tr(i18n.Duration), tr(i18n.SecondsLabel, tr(duration)), &draft.duration)
		settingInput(c, tr(i18n.AdvanceNotice), tr(i18n.SecondsLabel, tr(notice)), &draft.notice)
		settingInput(c, tr(i18n.PostponeBy), tr(i18n.SecondsLabel, tr(postpone)), &draft.postpone)
		ui.Checkbox(c, &draft.strict, tr(strict))
	})
}

func settingInput(c *ui.Context, label, accessibleLabel string, value *string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(112).Wrap()
		ui.TextInput(c, value).Label(accessibleLabel).Width(78).Direction(ui.LeftToRight)
	})
}

func viewDirection(language i18n.Language) ui.Direction {
	if language.RightToLeft() {
		return ui.RightToLeft
	}
	return ui.LeftToRight
}

func (a *desktop) breakView(c *ui.Context) {
	state := a.state
	if state.Phase != breaks.Active {
		return
	}
	tr := a.language.Text
	idea := i18n.LongIdea
	if state.Kind == breaks.Short {
		idea = i18n.ShortIdea
	}
	ui.Column(c).Fill().Direction(viewDirection(a.language)).Center().Gap(16).Padding(26).Children(func() {
		ui.Text(c, tr(breakLabel(state.Kind))).FontSize(24).Bold()
		ui.Text(c, countdown(state.Remaining)).FontSize(52).Bold().Direction(ui.LeftToRight)
		ui.Progress(c, float64(state.Remaining)/float64(state.Duration)).Width(280)
		ui.Text(c, tr(idea)).TextColor(c.Theme().TextMuted).TextAlign(ui.Center).Wrap()
		if !state.CanSkip {
			ui.Text(c, tr(i18n.StrictOn)).TextColor(c.Theme().TextMuted)
			return
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, tr(i18n.Postpone)).Disabled(!state.CanPostpone).Clicked() {
				a.postpone()
			}
			if ui.Button(c, tr(i18n.SkipBreak)).Clicked() {
				a.skip()
			}
		})
	})
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
