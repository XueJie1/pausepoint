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
	"pausepoint/internal/prefs"
)

type settingsDraft struct {
	interval    string
	short, long ruleDraft
}

type ruleDraft struct {
	duration, notice, postpone string
	strict                     bool
}

func newDraft(settings breaks.Settings) settingsDraft {
	seconds := func(d time.Duration) string { return strconv.FormatInt(int64(d/time.Second), 10) }
	rule := func(r breaks.Rule) ruleDraft {
		return ruleDraft{seconds(r.Duration), seconds(r.Notice), seconds(r.Postpone), r.Strict}
	}
	return settingsDraft{seconds(settings.Interval), rule(settings.Short), rule(settings.Long)}
}

func (d settingsDraft) parse() (breaks.Settings, error) {
	var out breaks.Settings
	fields := []struct {
		label string
		value string
		out   *time.Duration
	}{
		{"Work interval", d.interval, &out.Interval},
		{"Short break duration", d.short.duration, &out.Short.Duration},
		{"Short break notice", d.short.notice, &out.Short.Notice},
		{"Short break postponement", d.short.postpone, &out.Short.Postpone},
		{"Long break duration", d.long.duration, &out.Long.Duration},
		{"Long break notice", d.long.notice, &out.Long.Notice},
		{"Long break postponement", d.long.postpone, &out.Long.Postpone},
	}
	for _, field := range fields {
		value, err := strconv.ParseUint(strings.TrimSpace(field.value), 10, 32)
		if err != nil || value > uint64(breaks.MaxDuration/time.Second) {
			return out, fmt.Errorf("%s must be a whole number between 0 and 86400", field.label)
		}
		*field.out = time.Duration(value) * time.Second
	}
	out.Short.Strict, out.Long.Strict = d.short.strict, d.long.strict
	return out, out.Validate()
}

func (a *desktop) saveSettings() {
	if a.state.Phase == breaks.Active || a.state.Phase == breaks.Sleeping {
		return
	}
	settings, err := a.draft.parse()
	if err != nil {
		a.settingsInfo = err.Error()
		return
	}
	if a.path == "" {
		a.settingsInfo = "Your user configuration directory is unavailable."
		return
	}
	if err := prefs.Save(a.path, settings); err != nil {
		log.Print("save settings: ", err)
		a.settingsInfo = "Could not save your settings."
		return
	}
	now := time.Now()
	a.scheduler.Reconfigure(settings, now)
	if settings != a.prefs {
		a.clearNotice()
	}
	a.prefs, a.draft = settings, newDraft(settings)
	firstConfirmation := !a.confirmed
	a.confirmed = true
	a.settingsInfo = "Settings saved."
	a.refresh(now)
	if firstConfirmation && a.tray != nil {
		a.settings.Hide()
	}
}

func (a *desktop) settingsView(c *ui.Context) {
	theme := c.Theme()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().MaxWidth(680).AlignSelf(ui.Center).Padding(26).Gap(18).Children(func() {
			ui.Column(c).Gap(7).Children(func() {
				ui.Text(c, "Break reminders").FontSize(27).Bold()
				ui.Text(c, statusText(a.state)).FontSize(17).TextColor(theme.TextMuted)
				ui.Row(c).Gap(8).Children(func() {
					if ui.Button(c, pauseLabel(a.state)).Disabled(!canTogglePause(a.state)).Clicked() {
						a.togglePause()
					}
					if ui.Button(c, "Start next break now").Disabled(a.state.Phase != breaks.Waiting).Clicked() {
						a.startBreak()
					}
				})
			})
			if a.notifyInfo != "" {
				ui.Text(c, a.notifyInfo).TextColor(theme.TextMuted).Wrap()
			}
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "Schedule").FontSize(18).Bold()
				settingInput(c, "Work interval", "Work interval", &a.draft.interval)
				ui.Text(c, "A long break follows every two short breaks. All times are in seconds.").TextColor(theme.TextMuted).Wrap()
			})
			ui.Row(c).FillWidth().Gap(20).Children(func() {
				ruleFields(c, "Short break", &a.draft.short)
				ruleFields(c, "Long break", &a.draft.long)
			})
			ui.Text(c, "Set notice or postponement to 0 to turn it off. Strict mode requires the break to finish.").TextColor(theme.TextMuted).Wrap()
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				saveDisabled := a.state.Phase == breaks.Active || a.state.Phase == breaks.Sleeping
				saveLabel := "Save settings"
				if !a.confirmed {
					saveLabel = "Confirm settings"
				}
				if ui.PrimaryButton(c, saveLabel).Disabled(saveDisabled).Clicked() {
					a.saveSettings()
					c.Invalidate()
				}
				if ui.Button(c, "Restore defaults").Clicked() {
					a.draft = newDraft(breaks.Defaults())
					a.settingsInfo = "Confirm or save the settings to apply the defaults."
					c.Invalidate()
				}
				ui.Spacer(c).Grow(1)
				if ui.Button(c, "Close").Clicked() {
					a.settings.Hide()
				}
				if ui.Button(c, "Quit").Clicked() {
					mygo.App.Quit()
				}
			})
			if a.settingsInfo != "" {
				ui.Text(c, a.settingsInfo).TextColor(theme.TextMuted).Wrap()
			}
		})
	})
}

func ruleFields(c *ui.Context, title string, draft *ruleDraft) {
	ui.Column(c).Grow(1).Basis(0).Gap(6).Children(func() {
		ui.Text(c, title).FontSize(16).Bold()
		settingInput(c, "Duration", title+" duration", &draft.duration)
		settingInput(c, "Advance notice", title+" advance notice", &draft.notice)
		settingInput(c, "Postpone by", title+" postponement", &draft.postpone)
		ui.Checkbox(c, &draft.strict, "Strict mode for "+strings.ToLower(title)+"s")
	})
}

func settingInput(c *ui.Context, label, accessibleLabel string, value *string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(112)
		ui.TextInput(c, value).Label(accessibleLabel + " (seconds)").Width(78)
	})
}

func (a *desktop) breakView(c *ui.Context) {
	state := a.state
	if state.Phase != breaks.Active {
		return
	}
	short := state.Kind == breaks.Short
	idea := "Stand up and take a few steps."
	if short {
		idea = "Look away from the screen and relax your shoulders."
	}
	ui.Column(c).Fill().Center().Gap(16).Padding(26).Children(func() {
		ui.Text(c, state.Kind.Label()).FontSize(24).Bold()
		ui.Text(c, countdown(state.Remaining)).FontSize(52).Bold()
		ui.Progress(c, float64(state.Remaining)/float64(state.Duration)).Width(280)
		ui.Text(c, idea).TextColor(c.Theme().TextMuted).TextAlign(ui.Center).Wrap()
		if !state.CanSkip {
			ui.Text(c, "Strict mode is on.").TextColor(c.Theme().TextMuted)
			return
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Postpone").Disabled(!state.CanPostpone).Clicked() {
				a.postpone()
			}
			if ui.Button(c, "Skip break").Clicked() {
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

func statusText(s breaks.State) string {
	switch s.Phase {
	case breaks.Paused:
		return "Reminders paused"
	case breaks.Sleeping:
		return "Waiting for the system to wake"
	case breaks.Active:
		return s.Kind.Label() + " · " + countdown(s.Remaining) + " left"
	default:
		return s.Kind.Label() + " in " + countdown(s.Remaining)
	}
}

func pauseLabel(s breaks.State) string {
	if s.Phase == breaks.Paused {
		return "Resume reminders"
	}
	return "Pause reminders"
}

func canTogglePause(s breaks.State) bool {
	return s.Phase != breaks.Sleeping && (s.Phase != breaks.Active || s.CanSkip)
}
