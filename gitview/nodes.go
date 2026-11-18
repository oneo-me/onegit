package gitview

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// track subscribes the binding reading it to the view's changes, which
// invalidate announces.
func (v *View) track() {
	if v.rev != nil {
		v.rev.Get()
	}
}

// watch subscribes the binding reading it to the view's changes and to
// what its dialogs' fields hold.
func (v *View) watch() {
	v.track()
	if v.edits != nil {
		v.edits.Get()
	}
}

// invalidate shows the view's state again. Bindings read it through track,
// so it must not run while the view builds.
func (v *View) invalidate() {
	if v.rev != nil {
		v.rev.Update(func(n int) int { return n + 1 })
	}
	if v.opts.Invalidate != nil {
		v.opts.Invalidate()
	}
}

// edited announces a change a dialog's field made, which only the
// bindings reading watch follow, so typing builds nothing again.
func (v *View) edited() {
	if v.edits != nil {
		v.edits.Update(func(n int) int { return n + 1 })
	}
}

// dynamic builds its nodes again whenever the view changes.
func (v *View) dynamic(build func(cx *core.Scope) core.Node) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		v.track()
		var node core.Node
		cx.Untrack(func() { node = build(cx) })
		return node
	})
}

// region builds its nodes again only when key changes, so the controls
// inside keep their state as the rest of the view changes.
func region(cx *core.Scope, key func() string, build func(cx *core.Scope) core.Node) core.Node {
	k := core.NewMemo(cx, key)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		k.Get()
		var node core.Node
		cx.Untrack(func() { node = build(cx) })
		return node
	})
}

// is returns a binding reading the view's changes and then fn.
func (v *View) is(fn func() bool) func() bool {
	return func() bool {
		v.watch()
		return fn()
	}
}

// menuGroups collects a menu's items in groups, putting a line between
// groups that show items, so a group with every item left out leaves no
// line.
type menuGroups struct {
	items       []core.MenuItem
	shown, line bool
}

// group starts the next group.
func (g *menuGroups) group() { g.line = g.shown }

func (g *menuGroups) separate() {
	if g.line {
		g.items = append(g.items, core.MenuItem{Separator: true})
		g.line = false
	}
	g.shown = true
}

// item adds an action, which does nothing while disabled.
func (g *menuGroups) item(label string, disabled bool, fn func()) {
	g.separate()
	g.items = append(g.items, core.MenuItem{Label: label, Disabled: disabled, OnSelect: fn})
}

// shortcut adds an action that keys also run, as the menu shows them.
func (g *menuGroups) shortcut(label, keys string, disabled bool, fn func()) {
	g.separate()
	g.items = append(g.items, core.MenuItem{Label: label, Shortcut: keys, Disabled: disabled, OnSelect: fn})
}

// check adds an option, with a check while it is on.
func (g *menuGroups) check(label string, checked, disabled bool, fn func()) {
	g.separate()
	g.items = append(g.items, core.MenuItem{Label: label, Checkable: true, Checked: checked, Disabled: disabled, OnSelect: fn})
}

// submenu adds a submenu of items.
func (g *menuGroups) submenu(label string, disabled bool, items []core.MenuItem) {
	g.separate()
	g.items = append(g.items, core.MenuItem{Label: label, Disabled: disabled, Items: items})
}

// popup shows a menu the platform draws where a secondary click asked for
// it, unless it has no items. Menus are built as they open, from what the
// view holds then.
func (v *View) popup(cx *core.Scope, at core.Point, items []core.MenuItem) {
	if len(items) > 0 {
		desktop.PopupMenu(cx, items, at, nil)
	}
}

// menuOn is a view's handler of a secondary click opening the menu items
// builds.
func (v *View) menuOn(cx *core.Scope, items func() []core.MenuItem) func(core.ContextMenuEvent) {
	return func(e core.ContextMenuEvent) { v.popup(cx, e.PagePosition, items()) }
}

// toast shows a failure: as a toast through the host's toaster, and kept
// as the last one for a host without it.
func (v *View) toast(title string, err error) {
	v.lastToast = toastContent{title: title, description: err.Error()}
	if v.opts.Toaster != nil {
		v.opts.Toaster.Show(ui.ToastContent{Type: ui.ToastError, Title: title, Description: err.Error()})
	}
}

// toastContent is the last failure the view showed.
type toastContent struct{ title, description string }

// muted is text in the muted color that wraps.
func muted(cx *core.Scope, text string, size core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{Content: text, Style: core.Style{FontSize: size}, StyleFunc: ui.Colored(colors.Muted)}
}

// mutedf is muted text that follows fn.
func mutedf(cx *core.Scope, fn func() string, size core.Points) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: size}, StyleFunc: ui.Colored(colors.Muted)}
}

// colored is text in the color of role that wraps.
func colored(text string, size core.Points, color func() core.Color) core.Node {
	return core.Text{Content: text, Style: core.Style{FontSize: size}, StyleFunc: ui.Colored(color)}
}

// line is a single line of text, cut with an ellipsis where it runs out of
// room.
func line(text string, size core.Points, weight core.FontWeight, color func() core.Color) core.Text {
	t := ui.Text(text, size, weight, color)
	t.MaxLines = 1
	t.Style.FlexShrink, t.Style.MinWidth = 1, core.Pt(0)
	return t
}

// centered fills its place and centers its children, as an empty pane's
// message or spinner.
func centered(children ...core.Node) core.Node {
	return core.View{Style: ui.FillRest.Merge(core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(16)}),
		Children: children}
}

// spinner is a small ring of work in progress.
func spinner(size core.Points) core.Node { return ui.Spinner{Size: size} }

// hairline is a line across its parent, as between panes.
func hairline(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Height: core.Pt(1), FlexShrink: 0, AlignSelf: core.AlignStretch},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Border()} }}
}

// row lays out children in a row, centered across.
func row(gap core.Points, children ...core.Node) core.View {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: gap}, Children: children}
}

// column stacks children.
func column(gap core.Points, children ...core.Node) core.View {
	return core.View{Style: core.Style{Gap: gap}, Children: children}
}

// reason is why a dialog or popup asks to close, which the view ignores.
type reason = base.Reason
