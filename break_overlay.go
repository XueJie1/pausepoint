package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"pausepoint/internal/breaks"
	"pausepoint/internal/i18n"
	"pausepoint/internal/prefs"
)

const breakWindowWidth, breakWindowHeight = 460, 330

type breakOverlayPalette struct {
	canvas, field, border, text, muted, hover, pressed, progress, progressText, focus ui.Color
	dark                                                                              bool
}

func breakOverlayColors(choice prefs.Theme, systemDark bool) breakOverlayPalette {
	if choice == prefs.ThemeLight || (choice == prefs.ThemeSystem && !systemDark) {
		return breakOverlayPalette{
			canvas: ui.Hex("#F6F6F7"), field: ui.Hex("#F4F4F6"), border: ui.Hex("#DCDCE2"),
			text: ui.Hex("#18181B"), muted: ui.Hex("#5E5E69"), hover: ui.Hex("#EBEBEF"), pressed: ui.Hex("#DEDEE6"),
			progress: ui.Hex("#18181B"), progressText: ui.Hex("#FFFFFF"), focus: ui.Hex("#C73747"),
		}
	}
	return breakOverlayPalette{
		canvas: ui.Hex("#0D0D0E"), field: ui.Hex("#101012"), border: ui.Hex("#2A2A2E"),
		text: ui.Hex("#F4F4F5"), muted: ui.Hex("#A1A1AB"), hover: ui.Hex("#202024"), pressed: ui.Hex("#27272A"),
		progress: ui.Hex("#EDEDEF"), progressText: ui.Hex("#0D0D0E"), focus: ui.Hex("#FF6363"), dark: true,
	}
}

func (p breakOverlayPalette) theme() *ui.Theme {
	t := ui.LightTheme()
	if p.dark {
		t = ui.DarkTheme()
	}
	t.Background, t.Surface, t.Border = p.canvas, p.field, p.border
	t.SurfaceHover, t.SurfacePressed = p.hover, p.pressed
	t.Text, t.TextMuted = p.text, p.muted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.progress, p.progress, p.progress, p.progressText
	t.Focus, t.Selection, t.FontSize = p.focus, p.focus.Alpha(.25), 13
	return t
}

var breakOverlayMotion = ui.Animation{Duration: 150 * time.Millisecond, Ease: ui.CubicBezier(.2, 0, 0, 1)}

func breakOverlayButton(c *ui.Context, p breakOverlayPalette, label string, disabled bool, action func()) {
	button := ui.ButtonBase(c).Label(label).Disabled(disabled).Height(28).PaddingX(8).Radius(7).
		TextColor(p.text).FocusRingColor(p.focus).Border(1, p.border)
	fill := p.canvas.Alpha(0)
	if button.Hovered() && !disabled {
		fill = p.hover
	}
	button.Background(button.AnimateColor("fill", fill, breakOverlayMotion)).
		Children(func() { ui.Text(c, label) }).OnClick(action)
}

func (a *desktop) breakView(c *ui.Context) {
	a.renderBreak(c, a.state, false)
}

func (a *desktop) renderBreak(c *ui.Context, state breaks.State, preview bool) {
	if state.Phase != breaks.Active {
		return
	}
	tr := a.language.Text
	p := breakOverlayColors(a.prefs.Theme, c.Theme().Dark)
	c.SetTheme(p.theme())
	idea := i18n.LongIdea
	if state.Kind == breaks.Short {
		idea = i18n.ShortIdea
	}
	ui.Column(c).Fill().Direction(viewDirection(a.language)).Center().Gap(14).Padding(24).Background(p.canvas).Children(func() {
		ui.Text(c, tr(breakLabel(state.Kind))).FontSize(17).FontWeight(600).TextColor(p.text)
		ui.Text(c, countdown(state.Remaining)).FontSize(52).FontWeight(600).FontFeatures("tnum").TextColor(p.text).Direction(ui.LeftToRight)
		ui.Progress(c, float64(state.Remaining)/float64(state.Duration)).Width(250)
		ui.Text(c, tr(idea)).FontSize(13).TextColor(p.muted).TextAlign(ui.Center).Wrap()
		if !state.CanSkip {
			ui.Text(c, tr(i18n.StrictOn)).TextColor(p.muted)
		}
		if preview {
			breakOverlayButton(c, p, tr(i18n.Close), false, a.hidePreview)
		} else if state.CanSkip {
			ui.Row(c).Gap(8).Children(func() {
				breakOverlayButton(c, p, tr(i18n.Postpone), !state.CanPostpone, a.postpone)
				breakOverlayButton(c, p, tr(i18n.SkipBreak), false, a.skip)
			})
		}
	})
}
