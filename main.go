package main

import (
	"context"
	_ "embed"
	"log"
	"os"
	"slices"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
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
	scheduler    *breaks.Scheduler
	prefs        breaks.Settings
	path         string
	draft        settingsDraft
	settingsInfo string
	notifyInfo   string
	lastTick     time.Time
	state        breaks.State
	quitting     bool
	confirmed    bool
	openSettings bool

	settings, rest *mygo.Window
	tray           *mygo.Tray
	trayStatus     *mygo.MenuItem
	trayPause      *mygo.MenuItem
	trayStart      *mygo.MenuItem
	trayText       string

	notification    *mygo.Notification
	cancelNotice    context.CancelFunc
	completionSound *audio.Sound
}

func main() {
	mygo.App.SetName("Pausepoint")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	settings := breaks.Defaults()
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
		a.settingsInfo = "Could not load your settings. Defaults are in use."
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
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *desktop) ready() {
	sound, err := audio.DecodeSound(completionWAV)
	if err != nil {
		log.Print("decode break completion sound: ", err)
	} else {
		a.completionSound = sound
	}
	a.lastTick = time.Now()
	a.scheduler = breaks.New(a.prefs, a.lastTick)
	a.state = a.scheduler.State(a.lastTick)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.Power.OnSuspend(func() {
		a.scheduler.Suspend()
		a.refresh(time.Now())
	})
	mygo.Power.OnResume(func() { a.wake(time.Now()) })
	release := mygo.Power.KeepRunning("Schedule break reminders")

	a.settings = mygo.NewWindow(mygo.WindowOptions{
		Title: "Pausepoint", Width: 610, Height: 590, MinWidth: 540, MinHeight: 440,
		Hidden: true, Content: ui.View(a.settingsView),
	})
	a.settings.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting {
			return
		}
		e.PreventDefault()
		a.settings.Hide()
	})
	a.rest = mygo.NewWindow(mygo.WindowOptions{
		Title: "Pausepoint Break", Width: 460, Height: 330, Hidden: true,
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
	a.refresh(a.lastTick)
	if a.openSettings || !a.confirmed || a.tray == nil {
		a.showSettings()
	}
	done := make(chan struct{})
	mygo.App.OnQuit(func() {
		close(done)
		a.clearNotice()
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
	a.trayStatus = &mygo.MenuItem{Label: statusText(a.state), Disabled: true}
	a.trayPause = &mygo.MenuItem{Label: "Pause reminders", Click: menuAction(a.togglePause)}
	a.trayStart = &mygo.MenuItem{Label: "Start next break now", Click: menuAction(a.startBreak)}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID: "pausepoint", Icon: trayIcon, ToolTip: "Pausepoint",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			a.trayStatus, a.trayPause, a.trayStart, mygo.Separator(),
			{Label: "Settings…", Click: menuAction(a.showSettings)},
			{Role: mygo.RoleQuit},
		}),
	})
	if err != nil {
		log.Print("create tray: ", err)
		return
	}
	a.tray = tray
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory) // macOS uses the menu bar item as the main entry point.
}

func menuAction(fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) { fn() }
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

func (a *desktop) tick(now time.Time) {
	// Wall time includes system sleep even where Go's monotonic clock stops.
	wallGap := now.Round(0).Sub(a.lastTick.Round(0))
	if wallGap > 15*time.Second || wallGap < -time.Second {
		a.wake(now)
		return
	}
	a.lastTick = now
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
	a.lastTick = now
	a.clearNotice()
	a.scheduler.Wake(now)
	a.refresh(now)
}

func (a *desktop) refresh(now time.Time) {
	previous := a.state
	a.state = a.scheduler.State(now)
	if a.state.Phase != breaks.Waiting {
		a.clearNotice()
	}
	if a.state.Phase == breaks.Active && previous.Phase != breaks.Active {
		a.rest.Show()
	} else if a.state.Phase != breaks.Active && previous.Phase == breaks.Active {
		a.rest.Hide()
	}
	if a.state.Phase == breaks.Active {
		a.rest.Invalidate()
	}
	text := statusText(a.state)
	if a.tray != nil {
		if text != a.trayText {
			a.trayText = text
			a.trayStatus.SetLabel(text)
			a.tray.SetToolTip("Pausepoint · " + text)
		}
		if previous.Phase != a.state.Phase || previous.CanSkip != a.state.CanSkip {
			a.trayPause.SetLabel(pauseLabel(a.state))
			a.trayPause.SetEnabled(canTogglePause(a.state))
			a.trayStart.SetEnabled(a.state.Phase == breaks.Waiting)
		}
	}
	if a.settings.IsVisible() && (text != statusText(previous) || previous.CanSkip != a.state.CanSkip) {
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
	a.clearNotice()
	note := mygo.NewNotification(mygo.NotificationOptions{
		Title: "Pausepoint", Body: statusText(a.state),
	})
	ctx, cancel := context.WithTimeout(context.Background(), a.state.Remaining)
	a.notification, a.cancelNotice = note, cancel
	go func() {
		err := note.ShowContext(ctx)
		if err != nil && ctx.Err() == nil {
			log.Print("advance notice: ", err)
			mygo.RunOnMain(func() {
				if a.notification == note && !a.quitting {
					a.notifyInfo = "Desktop notifications are unavailable. Break windows will still open."
					a.settings.Invalidate()
				}
			})
		}
	}()
}

func (a *desktop) clearNotice() {
	if a.notification != nil {
		a.cancelNotice()
		a.notification.Close()
		a.notification = nil
		a.cancelNotice = nil
	}
}
