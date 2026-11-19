package main

import (
	"fmt"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

type settingsPage uint8

const (
	generalPage settingsPage = iota
	schedulePage
	appearancePage
	shortcutsPage
)

const (
	settingsHeadingFont = 20
	settingsBodyFont    = 13
	settingsSmallFont   = 12
	settingsCaptionFont = 11

	settingsPageGap       = 18
	settingsRowGap        = 12
	settingsSummaryGap    = 10
	settingsActionGap     = 8
	settingsInlineGap     = 6
	settingsNavGap        = 4
	settingsBadgeGap      = 3
	settingsLabelGap      = 2
	settingsCardRadius    = 10
	settingsButtonRadius  = 7
	settingsFieldRadius   = 6
	settingsSegmentRadius = 5
	settingsBadgeRadius   = 4

	settingsPagePadY     = 24
	settingsPagePadX     = 32
	settingsTitlePadX    = 12
	settingsSidebarPadY  = 12
	settingsSidebarPadX  = 10
	settingsStatusPadY   = 4
	settingsStatusPadX   = 8
	settingsRowPadY      = 10
	settingsRowPadX      = 16
	settingsSummaryPadY  = 14
	settingsButtonPadX   = 8
	settingsPreviewPadX  = 10
	settingsSegmentPadX  = 7
	settingsRecorderPadX = 5
	settingsBadgePadY    = 2
	settingsBadgePadX    = 4
	settingsFieldPadX    = 8

	settingsSwitchWidth  = 34
	settingsSwitchHeight = 20
	settingsSwitchInset  = 3
	settingsSwitchThumb  = settingsSwitchHeight - 2*settingsSwitchInset
	settingsSwitchTravel = settingsSwitchWidth - 2*settingsSwitchInset - settingsSwitchThumb
)

type settingsPalette struct {
	switchThumb                                                                                                  ui.Color
	canvas, sidebar, surface, border, divider, text, muted, hover, selected, accent, field, primary, primaryText ui.Color
	dark                                                                                                         bool
}

func settingsColors(choice prefs.Theme, systemDark bool) settingsPalette {
	if choice == prefs.ThemeLight || (choice == prefs.ThemeSystem && !systemDark) {
		return settingsPalette{
			switchThumb: ui.Hex("#F4F4F5"), canvas: ui.Hex("#F6F6F7"), sidebar: ui.Hex("#EEEEF1"), surface: ui.Hex("#FFFFFF"),
			border: ui.Hex("#DCDCE2"), divider: ui.Hex("#E9E9ED"), text: ui.Hex("#18181B"), muted: ui.Hex("#5E5E69"),
			hover: ui.Hex("#EBEBEF"), selected: ui.Hex("#DEDEE6"), accent: ui.Hex("#C73747"),
			field: ui.Hex("#F4F4F6"), primary: ui.Hex("#18181B"), primaryText: ui.Hex("#FFFFFF"),
		}
	}
	return settingsPalette{
		switchThumb: ui.Hex("#F4F4F5"), canvas: ui.Hex("#0D0D0E"), sidebar: ui.Hex("#121214"), surface: ui.Hex("#18181B"),
		border: ui.Hex("#2A2A2E"), divider: ui.Hex("#252529"), text: ui.Hex("#F4F4F5"), muted: ui.Hex("#A1A1AB"),
		hover: ui.Hex("#202024"), selected: ui.Hex("#27272A"), accent: ui.Hex("#FF6363"),
		field: ui.Hex("#101012"), primary: ui.Hex("#EDEDEF"), primaryText: ui.Hex("#0D0D0E"), dark: true,
	}
}

func (p settingsPalette) theme() *ui.Theme {
	t := ui.LightTheme()
	if p.dark {
		t = ui.DarkTheme()
	}
	t.Background, t.Surface, t.Border = p.canvas, p.field, p.border
	t.SurfaceHover, t.SurfacePressed = p.hover, p.selected
	t.Text, t.TextMuted = p.text, p.muted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.primary, p.primary, p.primary, p.primaryText
	t.Focus, t.Selection, t.FontSize = p.accent, p.accent.Alpha(.25), settingsBodyFont
	return t
}

func lineIcon(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" d="` + path + `"/></svg>`))
}

var (
	generalIcon    = lineIcon("M3 6h18M3 12h18M3 18h18M8 4v4m8 2v4m-6 2v4")
	scheduleIcon   = lineIcon("M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0ZM12 7v5l3 2")
	appearanceIcon = lineIcon("M4 5h16v12H4zM8 21h8m-4-4v4")
	shortcutsIcon  = lineIcon("M4 6h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1Zm3 4h1m3 0h2m3 0h1M8 14h8")
	startIcon      = lineIcon("M9 5l11 7-11 7z")
	settingsMotion = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.CubicBezier(.2, 0, 0, 1)}
)

func (a *desktop) settingsView(c *ui.Context) {
	tr := a.language.Text
	p := settingsColors(a.prefs.Theme, c.Theme().Dark)
	c.SetTheme(p.theme())
	a.previewShortcut(c)
	ui.Column(c).Fill().Background(p.canvas).Children(func() {
		settingsTitleBar(c, p)
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).FillWidth().AlignItems(ui.Stretch).Direction(viewDirection(a.language)).Children(func() {
			a.settingsSidebar(c, p)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				ui.Scroll(c).Key(a.settingsPage).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
					ui.Column(c).FillWidth().MaxWidth(744).AlignSelf(ui.Center).Padding(settingsPagePadY, settingsPagePadX).Gap(settingsPageGap).Children(func() {
						title := [...]i18n.Key{i18n.General, i18n.Schedule, i18n.Appearance, i18n.Shortcuts}[a.settingsPage]
						ui.Text(c, tr(title)).FontSize(settingsHeadingFont).FontWeight(600)
						switch a.settingsPage {
						case generalPage:
							a.generalView(c, p)
						case schedulePage:
							a.scheduleView(c, p)
						case appearancePage:
							a.appearanceView(c, p)
						case shortcutsPage:
							a.shortcutsView(c, p)
						}
					})
				})
				info, color := tr(i18n.AutoSaveHelp), p.muted
				if a.settingsLocked() {
					info = tr(i18n.SettingsLocked)
				}
				if a.settingsInfo.key != 0 && a.settingsInfo.key != i18n.Saved {
					info, color = a.settingsInfo.text(a.language), p.accent
				}
				ui.Column(c).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(p.divider).Padding(settingsRowPadY, settingsPagePadX).Children(func() {
					ui.Text(c, info).FontSize(settingsSmallFont).TextColor(color).Wrap()
				})
			})
		})
	})
}

func (a *desktop) updateWindowButtons() {
	if a.settings == nil {
		return
	}
	source := mygo.ThemeDark
	switch a.prefs.Theme {
	case prefs.ThemeLight:
		source = mygo.ThemeLight
	case prefs.ThemeSystem:
		source = mygo.ThemeSystem
	}
	a.settings.SetWindowButtonsTheme(source)
}

func settingsTitleBar(c *ui.Context, p settingsPalette) {
	bar := c.TitleBar()
	if runtime.GOOS == "darwin" {
		bar.Left = max(72, bar.Left)
	}
	ui.Row(c).FillWidth().Height(max(38, bar.Height)).Shrink(0).Direction(ui.LeftToRight).
		Padding(0, bar.Right+settingsTitlePadX, 0, bar.Left+settingsTitlePadX).AlignItems(ui.Center).Justify(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.divider).DragWindow().Children(func() {
		ui.Text(c, "Pausepoint").FontSize(settingsBodyFont).FontWeight(500)
	})
}

func (a *desktop) settingsSidebar(c *ui.Context, p settingsPalette) {
	tr := a.language.Text
	ui.Column(c).Width(200).Shrink(0).Padding(settingsSidebarPadY, settingsSidebarPadX).Gap(settingsNavGap).Background(p.sidebar).
		BorderEndWidth(1).BorderColor(p.border).Children(func() {
		for i, item := range [...]struct {
			label i18n.Key
			icon  *ui.SVG
		}{
			{i18n.General, generalIcon}, {i18n.Schedule, scheduleIcon},
			{i18n.Appearance, appearanceIcon}, {i18n.Shortcuts, shortcutsIcon},
		} {
			page := settingsPage(i)
			label := tr(item.label)
			button := settingsButton(c, p, label, a.settingsPage == page, false).FillWidth().Height(34).Justify(ui.Start).Gap(settingsActionGap)
			button.Children(func() {
				ui.Icon(c, item.icon).Size(15, 15).TextColor(p.muted)
				ui.Text(c, label).FontWeight(500).SingleLine()
			})
			button.OnClick(func() {
				a.flushSettings()
				a.settingsPage = page
				a.previewRecording = false
			})
		}
		ui.Spacer(c).Grow(1)
		ui.Text(c, statusText(a.language, a.state)).FontSize(settingsSmallFont).FontFeatures("tnum").TextColor(p.muted).Wrap().Padding(settingsStatusPadY, settingsStatusPadX)
		label := tr(i18n.StartBreakNow)
		settingsButton(c, p, label, false, a.state.Phase != breaks.Waiting).FillWidth().Height(34).Justify(ui.Start).Gap(settingsInlineGap).
			Children(func() {
				ui.Icon(c, startIcon).Size(15, 15)
				ui.Text(c, label).SingleLine().Tooltip(label)
			}).OnClick(func() {
			a.flushSettings()
			a.startBreak()
		})
	})
}

func settingsButton(c *ui.Context, p settingsPalette, label string, selected, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Label(label).Selected(selected).Disabled(disabled).Height(28).PaddingX(settingsButtonPadX).Radius(settingsButtonRadius).
		TextColor(p.text).FocusRingColor(p.accent)
	fill := p.canvas.Alpha(0)
	if selected {
		fill = p.selected
	} else if b.Hovered() && !disabled {
		fill = p.hover
	}
	return b.Background(b.AnimateColor("fill", fill, settingsMotion))
}

func settingsCard(c *ui.Context, p settingsPalette, children func()) {
	ui.Column(c).FillWidth().Background(p.surface).Border(1, p.border).Radius(settingsCardRadius).Children(children)
}

func settingsRow(c *ui.Context, p settingsPalette, title, help string, control func()) {
	ui.Row(c).FillWidth().MinHeight(52).Padding(settingsRowPadY, settingsRowPadX).Gap(settingsRowGap).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(settingsLabelGap).Children(func() {
			ui.Text(c, title).FontSize(settingsBodyFont).FontWeight(500).Wrap()
			if help != "" {
				ui.Text(c, help).FontSize(settingsSmallFont).TextColor(p.muted).SingleLine().Ellipsis("…").Tooltip(help)
			}
		})
		control()
	})
}

func settingsDivider(c *ui.Context, p settingsPalette) {
	ui.Column(c).Height(1).FillWidth().PaddingX(settingsRowPadX).Children(func() {
		ui.Column(c).Fill().Background(p.divider)
	})
}

func settingsSwitch(c *ui.Context, p settingsPalette, label string, on *bool, disabled bool, change func()) {
	control := ui.SwitchBase(c, on).Label(label).Disabled(disabled).Size(settingsSwitchWidth, settingsSwitchHeight).Shrink(0).FocusRingColor(p.accent)
	target, offset := p.selected, float32(0)
	if *on {
		target, offset = p.accent, settingsSwitchTravel
	}
	fill := control.AnimateColor("track", target, settingsMotion)
	x := control.AnimateTo("thumb", offset, settingsMotion)
	control.Draw(func(painter *ui.Painter, r ui.Rect) {
		painter.Fill(r, fill, settingsSwitchHeight/2)
		painter.Stroke(r, p.border, settingsSwitchHeight/2, 1)
		thumb := ui.Rect{X: r.X + settingsSwitchInset + x, Y: r.Y + settingsSwitchInset, W: settingsSwitchThumb, H: settingsSwitchThumb}
		painter.Fill(thumb, p.switchThumb, settingsSwitchThumb/2)
	}).OnChange(change)
}

func settingsSegmented(c *ui.Context, p settingsPalette, label string, options []string, selected *int, disabled bool, change func()) {
	parts := ui.SegmentedBase(c, selected, len(options))
	parts.Track.Label(label).Background(p.field).Border(1, p.border).Radius(settingsButtonRadius).
		Padding(settingsLabelGap).Gap(settingsLabelGap).Disabled(disabled).OnChange(change).Children(func() {
		for i, option := range options {
			segment := parts.Segment(i).MinHeight(24).PaddingX(settingsSegmentPadX).Radius(settingsSegmentRadius)
			if i == *selected {
				segment.Background(p.selected).Border(1, p.border)
			} else if segment.Hovered() {
				segment.Background(p.hover)
			}
			segment.Children(func() {
				ui.Text(c, option).FontSize(settingsSmallFont).TextColor(p.text).SingleLine()
			})
		}
	})
}

func settingsShortcutBadges(c *ui.Context, p settingsPalette, keys []string, primary bool) {
	background, border, text := p.field, p.border, p.text
	if primary {
		background, border, text = p.primaryText.Alpha(.07), p.primaryText.Alpha(.16), p.primaryText
	}
	ui.Row(c).Direction(ui.LeftToRight).Gap(settingsBadgeGap).Shrink(0).AlignItems(ui.Center).Children(func() {
		for _, key := range keys {
			ui.Text(c, key).FontSize(settingsCaptionFont).FontWeight(500).TextColor(text).SingleLine().
				Padding(settingsBadgePadY, settingsBadgePadX).Background(background).Border(1, border).Radius(settingsBadgeRadius)
		}
	})
}

func (a *desktop) generalView(c *ui.Context, p settingsPalette) {
	tr := a.language.Text
	settingsCard(c, p, func() {
		enabled := a.state.Phase != breaks.Paused && a.state.Phase != breaks.Sleeping
		settingsRow(c, p, tr(i18n.RemindersEnabled), statusText(a.language, a.state), func() {
			settingsSwitch(c, p, pauseLabel(a.language, a.state), &enabled, !canTogglePause(a.state), a.togglePause)
		})
		settingsDivider(c, p)
		settingsRow(c, p, tr(i18n.LanguageLabel), tr(i18n.LanguageHelp), func() { a.languageField(c) })
	})
	if a.notifyFailed {
		ui.Text(c, tr(i18n.NotificationsUnavailable)).TextColor(p.muted).FontSize(settingsSmallFont).Wrap()
	}
	ui.Row(c).Gap(settingsActionGap).Wrap().Children(func() {
		settingsButton(c, p, tr(i18n.RestoreDefaults), false, a.settingsLocked()).Border(1, p.border).
			Children(func() { ui.Text(c, tr(i18n.RestoreDefaults)) }).OnClick(a.restoreDefaultSettings)
		settingsButton(c, p, tr(i18n.Close), false, false).Border(1, p.border).
			Children(func() { ui.Text(c, tr(i18n.Close)) }).OnClick(a.closeSettings)
		settingsButton(c, p, tr(i18n.Quit), false, false).Border(1, p.border).
			Children(func() { ui.Text(c, tr(i18n.Quit)) }).OnClick(mygo.App.Quit)
	})
}

func (a *desktop) languageField(c *ui.Context) {
	tr := a.language.Text
	system := tr(i18n.SystemLanguage)
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
	ui.Select(c, &selected, options).Label(tr(i18n.LanguageLabel)).Disabled(a.settingsLocked()).OnChange(func() {
		a.draft.language = i18n.System
		for _, choice := range choices {
			if choice.Name == selected {
				a.draft.language = choice.Code
				break
			}
		}
		a.saveDiscreteSettings()
	})
}

func (a *desktop) scheduleView(c *ui.Context, p settingsPalette) {
	tr := a.language.Text
	settingsCard(c, p, func() {
		settings, validation := a.draft.parse()
		if validation.key != 0 {
			settings = a.prefs
		}
		ui.Row(c).FillWidth().Padding(settingsSummaryPadY, settingsRowPadX).Gap(settingsSummaryGap).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, scheduleIcon).Size(15, 15).TextColor(p.accent)
			ui.Text(c, tr(i18n.ScheduleSummary, summaryDuration(a.language, settings.Interval),
				summaryDuration(a.language, settings.Short.Duration), summaryDuration(a.language, settings.Long.Duration))).
				Grow(1).MinWidth(0).Wrap().FontFeatures("tnum")
		})
		settingsDivider(c, p)
		a.settingInput(c, p, i18n.WorkInterval, "", i18n.WorkInterval, &a.draft.interval)
	})
	a.ruleFields(c, p, breaks.Short, &a.draft.short)
	a.ruleFields(c, p, breaks.Long, &a.draft.long)
}

func summaryDuration(language i18n.Language, d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%d %s", d/time.Hour, language.Text(i18n.HoursUnit))
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%d %s", d/time.Minute, language.Text(i18n.MinutesUnit))
	}
	return fmt.Sprintf("%d %s", d/time.Second, language.Text(i18n.SecondsUnit))
}

func (a *desktop) ruleFields(c *ui.Context, p settingsPalette, kind breaks.Kind, draft *ruleDraft) {
	tr := a.language.Text
	duration, notice, postpone, strict := i18n.ShortDuration, i18n.ShortNotice, i18n.ShortPostpone, i18n.ShortStrict
	if kind == breaks.Long {
		duration, notice, postpone, strict = i18n.LongDuration, i18n.LongNotice, i18n.LongPostpone, i18n.LongStrict
	}
	ui.Column(c).Gap(settingsActionGap).Children(func() {
		ui.Text(c, tr(breakLabel(kind))).FontSize(settingsCaptionFont).FontWeight(600).LetterSpacing(.44).TextColor(p.muted)
		settingsCard(c, p, func() {
			a.settingInput(c, p, i18n.Duration, tr(i18n.DurationHelp), duration, &draft.duration)
			settingsDivider(c, p)
			a.settingInput(c, p, i18n.AdvanceNotice, tr(i18n.NoticeHelp), notice, &draft.notice)
			settingsDivider(c, p)
			a.settingInput(c, p, i18n.PostponeBy, tr(i18n.PostponeHelp), postpone, &draft.postpone)
			settingsDivider(c, p)
			settingsRow(c, p, tr(i18n.StrictMode), tr(i18n.StrictHelp), func() {
				settingsSwitch(c, p, tr(strict), &draft.strict, a.settingsLocked(), a.saveDiscreteSettings)
			})
		})
	})
}

func (a *desktop) settingInput(c *ui.Context, p settingsPalette, label i18n.Key, help string, accessible i18n.Key, value *string) {
	tr := a.language.Text
	settingsRow(c, p, tr(label), help, func() {
		ui.Row(c).Shrink(0).Height(28).AlignItems(ui.Center).Background(p.field).Border(1, p.border).Radius(settingsFieldRadius).Children(func() {
			ui.TextInputBase(c, value).Label(tr(i18n.SecondsLabel, tr(accessible))).Width(64).FillHeight().Padding(0, settingsFieldPadX).
				Direction(ui.LeftToRight).FontFeatures("tnum").FocusRingColor(p.accent).Disabled(a.settingsLocked()).
				OnChange(a.scheduleSettingsSave).OnSubmit(a.flushSettings)
			ui.Text(c, tr(i18n.SecondsUnit)).FontSize(settingsSmallFont).TextColor(p.muted).PaddingEnd(settingsFieldPadX)
		})
	})
}
