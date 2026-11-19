package main

import (
	_ "embed"
	"log"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

//go:embed resources/icon.png
var appIcon []byte

//go:embed resources/tray.png
var trayIcon []byte

//go:embed resources/sound/din-ding.wav
var completionWAV []byte

const (
	tickInterval = 250 * time.Millisecond
	settingsArg  = "--settings"
)

// All desktop state is owned by the MyGo main thread.
type desktop struct {
	scheduler       *breaks.Scheduler
	prefs           prefs.Settings
	language        i18n.Language
	path            string
	draft           settingsDraft
	settingsInfo    formMessage
	settingsPage    settingsPage
	saveAfter       time.Time
	settingsPending bool
	discretePending bool
	notifyFailed    bool
	state           breaks.State
	quitting        bool
	confirmed       bool
	openSettings    bool

	settings, rest, preview *mygo.Window
	previewUntil            time.Time
	previewRecording        bool
	previewShortcutFocus    ui.Handle
	tray                    *mygo.Tray
	trayStatus              *mygo.MenuItem
	trayPause               *mygo.MenuItem
	trayStart               *mygo.MenuItem
	traySettings            *mygo.MenuItem
	trayQuit                *mygo.MenuItem
	trayText                string

	notifications   *noticeWorker
	notification    *breakNotice
	completionSound *audio.Sound
}

func main() {
	mygo.App.SetName("Pausepoint")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	settings := prefs.Defaults()
	var confirmed bool
	path, err := prefs.Path()
	if err == nil {
		settings, confirmed, err = prefs.Load(path)
	}
	a := &desktop{
		prefs: settings, path: path, draft: newDraft(settings), confirmed: confirmed,
		openSettings: slices.Contains(os.Args[1:], settingsArg),
	}
	if err != nil {
		log.Print("load settings: ", err)
		a.settingsInfo = formMessage{key: i18n.LoadFailed}
	}
	mygo.App.WhenReady(a.ready)
	mygo.App.OnActivate(func(visible bool) {
		if !visible && (!a.confirmed || a.tray == nil) {
			a.showSettings()
		}
	})
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if slices.Contains(args, settingsArg) {
			a.openSettings = true
		}
		if a.openSettings || !a.confirmed || a.tray == nil {
			a.showSettings()
		}
	})
	err = mygo.App.Run()
	if a.notifications != nil {
		a.notifications.stop()
		<-a.notifications.done
	}
	if err != nil {
		log.Fatal(err)
	}
}

func (a *desktop) ready() {
	a.language = resolveLanguage(a.prefs.Language, mygo.App.PreferredLanguages())
	sound, err := audio.DecodeSound(completionWAV)
	if err != nil {
		log.Print("decode break completion sound: ", err)
	} else {
		a.completionSound = sound
	}
	now := time.Now()
	a.scheduler = breaks.New(a.prefs.Settings, now)
	a.state = a.scheduler.State(now)
	mygo.App.OnBeforeQuit(a.beforeQuit)
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.Power.OnSuspend(func() {
		a.scheduler.Suspend()
		a.refresh(time.Now())
	})
	mygo.Power.OnResume(func() { a.wake(time.Now()) })
	release := mygo.Power.KeepRunning(a.language.Text(i18n.KeepRunningReason))

	a.settings = mygo.NewWindow(mygo.WindowOptions{
		Title: "Pausepoint", Width: 860, Height: 660, MinWidth: 680, MinHeight: 460,
		TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 38,
		Hidden: true, Content: ui.View(a.settingsView),
	})
	a.settings.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.closeSettings()
	})
	a.settings.OnHide(func() {
		a.previewRecording = false
		a.hidePreview()
	})
	a.updateWindowButtons()
	a.rest = mygo.NewWindow(mygo.WindowOptions{
		Title: a.language.Text(i18n.BreakWindowTitle), Width: breakWindowWidth, Height: breakWindowHeight, Hidden: true,
		AlwaysOnTop: true, SkipTaskbar: true, DisableMinimize: true, DisableMaximize: true,
		Content: ui.View(a.breakView),
	})
	for _, window := range []*mygo.Window{a.settings, a.rest} {
		if err := window.SetIcon(appIcon); err != nil {
			log.Print("set window icon: ", err)
		}
	}
	a.rest.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.skip()
		}
	})

	a.installTray()
	a.refresh(now)
	if a.openSettings || !a.confirmed || a.tray == nil {
		a.showSettings()
	}
	a.notifications = newNoticeWorker(newBreakNotifier(), func(notice *breakNotice) {
		mygo.RunOnMain(func() { a.noticeFailed(notice) })
	})
	done := make(chan struct{})
	mygo.App.OnQuit(func() {
		close(done)
		a.notification = nil
		a.notifications.stop()
		if backend, ok := a.notifications.backend.(interface{ closeOnQuit() }); ok {
			backend.closeOnQuit()
		}
		release()
	})
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mygo.RunOnMain(func() {
					if !a.quitting {
						a.tick(time.Now())
					}
				})
			}
		}
	}()
}

func (a *desktop) installTray() {
	a.trayStatus = &mygo.MenuItem{Label: statusText(a.language, a.state), Disabled: true}
	a.trayPause = &mygo.MenuItem{Label: pauseLabel(a.language, a.state), Click: menuAction(a.togglePause)}
	a.trayStart = &mygo.MenuItem{Label: a.language.Text(i18n.StartBreakNow), Click: menuAction(a.startBreak)}
	a.traySettings = &mygo.MenuItem{Label: a.language.Text(i18n.TraySettings), Click: menuAction(a.showSettings)}
	a.trayQuit = &mygo.MenuItem{Label: a.language.Text(i18n.Quit), Role: mygo.RoleQuit}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID: "pausepoint", Icon: trayIcon, ToolTip: "Pausepoint",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			a.trayStatus, a.trayPause, a.trayStart, mygo.Separator(), a.traySettings, a.trayQuit,
		}),
	})
	if err != nil {
		log.Print("create tray: ", err)
		return
	}
	a.tray = tray
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory) // macOS uses the menu bar item as the main entry point.
}

func (a *desktop) updateLanguage() {
	var preferred []string
	if a.prefs.Language == i18n.System && a.settings != nil {
		preferred = mygo.App.PreferredLanguages()
	}
	language := resolveLanguage(a.prefs.Language, preferred)
	if language == a.language {
		return
	}
	a.language = language
	if a.rest != nil {
		a.rest.SetTitle(language.Text(i18n.BreakWindowTitle))
	}
	if a.tray != nil {
		a.trayPause.SetLabel(pauseLabel(language, a.state))
		a.trayStart.SetLabel(language.Text(i18n.StartBreakNow))
		a.traySettings.SetLabel(language.Text(i18n.TraySettings))
		a.trayQuit.SetLabel(language.Text(i18n.Quit))
	}
	if a.settings != nil {
		a.settings.Invalidate()
	}
}

func menuAction(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) { fn() }
}

func (a *desktop) closeSettings() {
	a.flushSettings()
	if !a.settingsLocked() && !a.confirmed && a.settingsInfo.key == 0 && a.draft == newDraft(a.prefs) {
		a.settingsPending = true
		a.commitPendingSettings(time.Now())
	}
	a.settings.Hide()
}

func (a *desktop) showSettings() {
	if a.settings == nil || a.quitting {
		return
	}
	a.openSettings = false
	if a.settings.IsMinimized() {
		a.settings.Restore()
	}
	a.settings.Show()
}

func (a *desktop) beforeQuit(e *mygo.QuitEvent) {
	// Pending edits were accepted before the UI was locked.
	if !a.commitPendingSettings(time.Now()) && !e.SessionEnding {
		e.PreventDefault()
		a.showSettings()
		return
	}
	a.quitting = true
}

func (a *desktop) tick(now time.Time) {
	if !a.settingsLocked() && !a.saveAfter.IsZero() && !now.Before(a.saveAfter) {
		a.commitPendingSettings(now)
	}
	// time.Now carries monotonic time. Advance handles missed ticks without
	// treating a delayed frame or a wall-clock adjustment as a power event.
	wasActive := a.state.Phase == breaks.Active
	notice := a.scheduler.Advance(now)
	a.refresh(now)
	if wasActive && a.state.Phase == breaks.Waiting && a.completionSound != nil {
		a.completionSound.Play()
	}
	if notice {
		a.showNotice()
	}
}

func (a *desktop) wake(now time.Time) {
	a.clearNotice()
	a.scheduler.Wake(now)
	a.refresh(now)
}

func (a *desktop) refresh(now time.Time) {
	previous := a.state
	a.state = a.scheduler.State(now)
	a.refreshPreview(now)
	if a.settingsLocked() {
		a.previewRecording = false
	}
	if a.state.Phase != breaks.Waiting {
		a.clearNotice()
	}
	if a.rest != nil {
		if a.state.Phase == breaks.Active {
			if previous.Phase != breaks.Active {
				a.rest.Show()
			}
			a.rest.Invalidate()
		} else if previous.Phase == breaks.Active {
			a.rest.Hide()
		}
	}
	text := statusText(a.language, a.state)
	if a.tray != nil {
		if text != a.trayText {
			a.trayText = text
			a.trayStatus.SetLabel(text)
			a.tray.SetToolTip("Pausepoint · " + text)
		}
		if previous.Phase != a.state.Phase || previous.CanSkip != a.state.CanSkip {
			a.trayPause.SetLabel(pauseLabel(a.language, a.state))
			a.trayPause.SetEnabled(canTogglePause(a.state))
			a.trayStart.SetEnabled(a.state.Phase == breaks.Waiting)
		}
	}
	if a.settings != nil && a.settings.IsVisible() && (text != statusText(a.language, previous) || previous.CanSkip != a.state.CanSkip) {
		a.settings.Invalidate()
	}
}

func (a *desktop) togglePause() {
	now := time.Now()
	if a.state.Phase == breaks.Paused {
		a.scheduler.Resume(now)
	} else {
		a.scheduler.Pause()
	}
	a.refresh(now)
}

func (a *desktop) startBreak() {
	now := time.Now()
	if a.scheduler.Start(now) {
		a.refresh(now)
	}
}

func (a *desktop) skip() {
	now := time.Now()
	if a.scheduler.Skip(now) {
		a.refresh(now)
	}
}

func (a *desktop) postpone() {
	now := time.Now()
	if a.scheduler.Postpone(now) {
		a.refresh(now)
	}
}

func (a *desktop) showNotice() {
	if a.notifications == nil || a.state.Remaining <= 0 {
		return
	}
	notice := &breakNotice{
		title: "Pausepoint", body: statusText(a.language, a.state),
		expires: time.Now().Add(a.state.Remaining), actions: []string{"default", "Open"},
	}
	a.notification = notice
	a.notifications.update(notice)
}

func (a *desktop) noticeFailed(notice *breakNotice) {
	if a.notification == notice && !a.quitting {
		a.notifyFailed = true
		if a.settings != nil {
			a.settings.Invalidate()
		}
	}
}

func (a *desktop) clearNotice() {
	if a.notification != nil {
		a.notification = nil
		a.notifications.update(nil)
	}
}
