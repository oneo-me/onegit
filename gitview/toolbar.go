package gitview

import (
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Toolbar is the view's Git actions for a host's title bar as raised
// buttons with an icon and name, size tall: fetch, and the pull and push
// dialogs followed by the commits a pull would bring and a push would send,
// and the stash dialog. Every action runs through the view.
func (v *View) Toolbar(size core.Points) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		v.Bind(cx)
		tr := func() Tracking { v.track(); return v.tracking }
		return row(6,
			v.toolbarAction(cx, OpFetch, lucide.Download, v.text("fetch"), size, nil, v.CanSync, v.OpenFetch),
			v.toolbarAction(cx, OpPull, lucide.ArrowDownToLine, v.text("pull"), size, func() int { return tr().Behind }, v.CanPull, v.OpenPull),
			v.toolbarAction(cx, OpPush, lucide.ArrowUpFromLine, v.text("push"), size, func() int { return tr().Ahead }, v.CanSync, v.OpenPush),
			v.toolbarAction(cx, OpStash, lucide.Archive, v.text("stash"), size, nil, v.CanStash, v.OpenStash),
		)
	})
}

// toolbarAction is the toolbar button for op: a raised capsule with its
// icon, label and, when count is set, the commits it moves in the muted
// color. A running action shows a spinner in place of its icon.
func (v *View) toolbarAction(cx *core.Scope, op string, icon func() *core.SVG, label string, size core.Points, count func() int, can func() bool, action func()) core.Node {
	colors := ui.ColorsOf(cx)
	running := core.NewMemo(cx, func() bool { v.track(); return v.Syncing() == op })
	glyph := core.Dynamic(func(cx *core.Scope) core.Node {
		if running.Get() {
			return spinner(16)
		}
		return ui.Icon(icon, 16, colors.Text)
	})
	children := []core.Node{glyph, line(label, 13, ui.Regular, colors.Text)}
	if count != nil {
		children = append(children, core.Show{When: func() bool { return count() > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return strconv.Itoa(count()) }, MaxLines: 1,
				Style: core.Style{FontSize: 13, FlexShrink: 0}, StyleFunc: ui.Colored(colors.Muted)}
		}})
	}
	disabled := v.is(func() bool { return !can() })
	button := base.Button{Label: label, TestID: "toolbar-" + op, Disabled: disabled,
		OnPress: func(core.PressEvent) {
			if can() {
				action()
			}
		},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(size), Padding: core.Symmetric(0, 12),
			CornerRadius: size / 2, BorderWidth: 1, FlexShrink: 0, Disabled: &core.Style{Opacity: core.Alpha(0.45)}},
		StyleFunc: func() core.Style { return raisedFace(cx) },
		Children:  children}
	tip := func() string { v.track(); return v.ActionTooltip(op) }
	return ui.Tooltip{TextFunc: tip, Trigger: button, Style: core.Style{FlexShrink: 0}}
}

// raisedFace is a raised toolbar control's look: the surface
// over a hairline, lifted by a faint shadow, darker under the pointer and
// held down.
func raisedFace(cx *core.Scope) core.Style {
	p := cx.Palette()
	face, hover, pressed, shadow := p.Surface, p.SurfaceHover, p.SurfacePressed, core.RGBA(0x000000, 0.14)
	if cx.Dark() {
		face, hover, shadow = p.SurfaceHover, p.SurfaceHover, core.RGBA(0x000000, 0.4)
	}
	return core.Style{Background: face, BorderColor: ui.Alpha(p.Border, 0.8), Color: p.Text,
		Shadows: []core.BoxShadow{ui.Shadow(0, 1, 2, 0, ui.Alpha(shadow, 0.5))},
		Hover:   &core.Style{Background: hover}, Active: &core.Style{Background: pressed}}
}
