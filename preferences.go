package main

import (
	"errors"
	"log"
	"time"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

func (a *desktop) settingsLocked() bool {
	return a.state.Phase == breaks.Active || a.state.Phase == breaks.Sleeping
}

func (a *desktop) scheduleSettingsSave() {
	a.settingsPending = true
	a.saveAfter = time.Now().Add(450 * time.Millisecond)
	a.scheduleTick()
}

func (a *desktop) saveDiscreteSettings() {
	if a.settingsLocked() {
		return
	}
	a.discretePending = true
	_, validation := a.draft.withNumbers(a.prefs)
	if a.persistPreferences(a.draft.withDiscrete(a.prefs), validation, time.Now()) {
		a.discretePending = false
	}
}

func (a *desktop) restoreDefaultSettings() {
	if a.settingsLocked() {
		return
	}
	a.draft = newDraft(prefs.Defaults())
	a.settingsPending, a.discretePending = true, true
	a.commitPendingSettings(time.Now())
}

func (a *desktop) flushSettings() {
	if !a.settingsLocked() {
		a.commitPendingSettings(time.Now())
	}
}

// commitPendingSettings also runs on quit, when the editor may be locked.
// Invalid numeric input cannot be saved, but pending discrete edits still can.
func (a *desktop) commitPendingSettings(now time.Time) bool {
	if !a.settingsPending && !a.discretePending {
		return true
	}
	a.saveAfter = time.Time{}
	candidate, validation := a.draft.withNumbers(a.prefs)
	if validation.key != 0 {
		a.settingsPending = false
	}
	if !a.settingsPending {
		candidate = a.prefs
	}
	if a.discretePending {
		candidate = a.draft.withDiscrete(candidate)
	}
	if !a.settingsPending && !a.discretePending {
		a.setSettingsInfo(validation)
		return true
	}
	if !a.persistPreferences(candidate, validation, now) {
		return false
	}
	a.settingsPending, a.discretePending = false, false
	return true
}

// persistPreferences applies the preferences once they are visible on disk.
// A directory sync error is reported separately because the rename succeeded.
func (a *desktop) persistPreferences(settings prefs.Settings, validation formMessage, now time.Time) bool {
	if a.confirmed && settings == a.prefs {
		a.setSettingsInfo(validation)
		return true
	}
	if a.path == "" {
		log.Print("save settings: configuration path unavailable")
		a.setSettingsInfo(formMessage{key: i18n.ConfigUnavailable})
		return false
	}
	message := validation
	if err := prefs.Save(a.path, settings); err != nil {
		log.Print("save settings: ", err)
		var durabilityErr *prefs.DurabilityError
		if !errors.As(err, &durabilityErr) {
			a.setSettingsInfo(formMessage{key: i18n.SaveFailed})
			return false
		}
		message = formMessage{key: i18n.SaveDurabilityUncertain}
	}
	a.applyPreferences(settings, now)
	if message.key == 0 {
		message.key = i18n.Saved
	}
	a.setSettingsInfo(message)
	return true
}

func (a *desktop) setSettingsInfo(message formMessage) {
	a.settingsInfo = message
	if a.settings != nil {
		a.settings.Invalidate()
	}
}

func (a *desktop) applyPreferences(settings prefs.Settings, now time.Time) {
	if settings.Settings != a.prefs.Settings {
		a.scheduler.Reconfigure(settings.Settings, now)
		a.clearNotice()
	}
	previousLanguage := a.language
	a.prefs = settings
	a.confirmed = true
	a.updateLanguage()
	a.refresh(now)
	if previousLanguage != a.language && a.notification != nil {
		a.showNotice()
	}
	a.updateWindowButtons()
}
