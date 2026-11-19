package main

import (
	"log"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

func (a *desktop) appearanceView(c *ui.Context, p settingsPalette) {
	tr := a.language.Text
	settingsCard(c, p, func() {
		ui.Row(c).FillWidth().Padding(settingsRowPadY, settingsRowPadX).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			preview := ui.ButtonBase(c).Label(tr(i18n.PreviewBreak)).MinHeight(30).MinWidth(0).
				Gap(settingsInlineGap).PaddingX(settingsPreviewPadX).Radius(settingsButtonRadius).AlignItems(ui.Center).FocusRingColor(p.accent).
				Disabled(a.settingsLocked()).OnClick(a.showPreview)
			fill := p.primary
			if preview.Hovered() {
				fill = fill.Mix(p.primaryText, .08)
			}
			preview.Background(preview.AnimateColor("preview", fill, settingsMotion)).Children(func() {
				ui.Icon(c, startIcon).Size(15, 15).Shrink(0).TextColor(p.primaryText)
				ui.Text(c, tr(i18n.PreviewBreak)).MinWidth(0).FontSize(settingsBodyFont).FontWeight(500).
					TextColor(p.primaryText).SingleLine().Ellipsis("…")
				a.shortcutBadges(c, p, true)
			})
		})
		settingsDivider(c, p)
		settingsRow(c, p, tr(i18n.ThemeLabel), tr(i18n.ThemeHelp), func() {
			themes := [...]prefs.Theme{prefs.ThemeSystem, prefs.ThemeDark, prefs.ThemeLight}
			labels := [...]i18n.Key{i18n.ThemeSystem, i18n.ThemeDark, i18n.ThemeLight}
			selected := 1
			for i, theme := range themes {
				if a.draft.theme == theme {
					selected = i
				}
			}
			var options [len(labels)]string
			for i, label := range labels {
				options[i] = tr(label)
			}
			settingsSegmented(c, p, tr(i18n.ThemeLabel), options[:], &selected, a.settingsLocked(), func() {
				a.draft.theme = themes[selected]
				a.saveDiscreteSettings()
			})
		})
	})
}

func effectivePreviewShortcut(shortcut prefs.Shortcut) prefs.Shortcut {
	if !shortcut.Valid() || shortcut.Key == "" {
		return prefs.Shortcut{Key: "B", Shift: true}
	}
	return shortcut
}

func (a *desktop) previewShortcut(c *ui.Context) {
	shortcut := effectivePreviewShortcut(a.prefs.PreviewShortcut)
	key := shortcut.Key[0]
	var uiKey ui.Key
	if key >= 'A' {
		uiKey = ui.KeyA + ui.Key(key-'A')
	} else {
		uiKey = ui.Key0 + ui.Key(key-'0')
	}
	mods := ui.Cmd
	if shortcut.Shift {
		mods |= ui.Shift
	}
	if shortcut.Alt {
		mods |= ui.Alt
	}
	c.OnWindowShortcut(mods, uiKey, func() {
		if !a.previewRecording && !c.ShortcutRepeated() {
			a.showPreview()
		}
	})
}

func (a *desktop) shortcutBadges(c *ui.Context, p settingsPalette, primary bool) {
	shortcut := effectivePreviewShortcut(a.draft.previewShortcut)
	cmd, shift, alt := "Ctrl", "Shift", "Alt"
	if runtime.GOOS == "darwin" {
		cmd, shift, alt = "⌘", "⇧", "⌥"
	}
	keys := []string{cmd}
	if shortcut.Alt {
		keys = append(keys, alt)
	}
	if shortcut.Shift {
		keys = append(keys, shift)
	}
	keys = append(keys, shortcut.Key)
	settingsShortcutBadges(c, p, keys, primary)
}

func (a *desktop) shortcutsView(c *ui.Context, p settingsPalette) {
	tr := a.language.Text
	settingsCard(c, p, func() {
		settingsRow(c, p, tr(i18n.PreviewBreak), tr(i18n.PreviewShortcutHelp), func() {
			ui.Row(c).Gap(settingsInlineGap).AlignItems(ui.Center).Wrap().Children(func() {
				label := i18n.RecordShortcut
				if a.previewRecording {
					label = i18n.RecordingShortcut
				}
				button := ui.ButtonBase(c).Label(tr(label)).Tooltip(tr(label)).MinHeight(28).
					PaddingX(settingsRecorderPadX).Radius(settingsFieldRadius).Center().Border(1, p.border).Background(p.surface).
					Disabled(a.settingsLocked()).Bind(&a.previewShortcutFocus).
					OnClick(func() {
						a.previewRecording = !a.previewRecording
						if a.previewRecording {
							a.previewShortcutFocus.Focus(c)
						}
						c.Invalidate()
					}).HandleInput(func(ev ui.InputEvent) bool {
					return a.recordPreviewShortcut(c, ev)
				})
				if button.FocusLeft() && a.previewRecording {
					a.cancelShortcutRecording(c)
				}
				button.Children(func() {
					if a.previewRecording {
						ui.Text(c, tr(label)).FontSize(settingsSmallFont).TextColor(p.text).Wrap()
					} else {
						a.shortcutBadges(c, p, false)
					}
				})
				if a.draft.previewShortcut.Key != "" {
					settingsButton(c, p, tr(i18n.ResetShortcut), false, a.settingsLocked()).Border(1, p.border).
						Children(func() { ui.Text(c, tr(i18n.ResetShortcut)) }).OnClick(func() {
						a.previewRecording = false
						a.draft.previewShortcut = prefs.Shortcut{}
						a.saveDiscreteSettings()
					})
				}
			})
		})
	})
}

func (a *desktop) recordPreviewShortcut(c *ui.Context, ev ui.InputEvent) bool {
	if !a.previewRecording || ev.Kind != ui.InputKeyDown || a.settingsLocked() {
		return false
	}
	if ev.Key == ui.KeyEscape || ev.Key == ui.KeyTab {
		a.cancelShortcutRecording(c)
		return ev.Key == ui.KeyEscape
	}
	if ev.Key == ui.KeyUnknown && ev.PhysicalKey == ui.KeyUnknown || ev.Repeat {
		return true
	}
	shortcut, valid := previewShortcutFromInput(ev)
	if !valid {
		a.settingsInfo = formMessage{key: i18n.ShortcutInvalid}
		c.Invalidate()
		return true
	}
	a.previewRecording = false
	a.draft.previewShortcut = shortcut
	a.saveDiscreteSettings()
	return true
}

func (a *desktop) cancelShortcutRecording(c *ui.Context) {
	a.previewRecording = false
	if a.settingsInfo.key == i18n.ShortcutInvalid {
		_, a.settingsInfo = a.draft.withNumbers(a.prefs)
	}
	c.Invalidate()
}

func previewShortcutFromInput(ev ui.InputEvent) (prefs.Shortcut, bool) {
	if ev.Mods&ui.Cmd == 0 || ev.Mods & ^(ui.Cmd|ui.Shift|ui.Alt) != 0 {
		return prefs.Shortcut{}, false
	}
	key := ev.Key
	if key < ui.Key0 || key > ui.KeyZ {
		key = ev.PhysicalKey
	}
	var char rune
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		char = rune(key-ui.KeyA) + 'A'
	case key >= ui.Key0 && key <= ui.Key9:
		char = rune(key-ui.Key0) + '0'
	default:
		return prefs.Shortcut{}, false
	}
	shortcut := prefs.Shortcut{Key: string(char), Shift: ev.Mods&ui.Shift != 0, Alt: ev.Mods&ui.Alt != 0}
	return shortcut, shortcut.Valid()
}

func (a *desktop) showPreview() {
	if a.settings == nil || a.quitting || a.settingsLocked() {
		return
	}
	if a.preview == nil {
		a.preview = mygo.NewWindow(mygo.WindowOptions{
			Width: breakWindowWidth, Height: breakWindowHeight, Hidden: true,
			Parent: a.settings, SkipTaskbar: true, DisableMinimize: true, DisableMaximize: true,
			Content: ui.View(a.previewView),
		})
		if err := a.preview.SetIcon(appIcon); err != nil {
			log.Print("set preview window icon: ", err)
		}
		a.preview.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.hidePreview()
			}
		})
	}
	a.previewUntil = time.Now().Add(breaks.Defaults().Short.Duration)
	a.preview.SetTitle(a.language.Text(i18n.PreviewTitle))
	a.preview.Show()
	a.preview.Invalidate()
}

func (a *desktop) hidePreview() {
	a.previewUntil = time.Time{}
	if a.preview != nil {
		a.preview.Hide()
	}
}

func (a *desktop) refreshPreview(now time.Time) {
	if a.previewUntil.IsZero() {
		return
	}
	if !now.Before(a.previewUntil) || a.settingsLocked() {
		a.hidePreview()
		return
	}
	a.preview.Invalidate()
}

func (a *desktop) previewView(c *ui.Context) {
	if a.previewUntil.IsZero() {
		return
	}
	c.OnWindowShortcut(0, ui.KeyEscape, a.hidePreview)
	duration := breaks.Defaults().Short.Duration
	a.renderBreak(c, breaks.State{
		Phase: breaks.Active, Kind: breaks.Short, Duration: duration,
		Remaining: min(duration, max(0, a.previewUntil.Sub(c.Now()))),
		CanSkip:   !a.prefs.Short.Strict,
	}, true)
}
