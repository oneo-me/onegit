package gitview

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Toolbar builds the view's Git actions for a host's title bar as raised
// buttons with an icon and name: fetch, and the pull and push dialogs followed
// by the commits a pull would bring and a push would send, and the stash
// dialog. Every action runs through the view.
func (v *View) Toolbar(c *ui.Context, size float32) {
	tr := v.tracking
	ui.Row(c).Key("gitview-toolbar").Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
		v.toolbarAction(c, OpFetch, lucide.Download, v.text("fetch"), size, 0, !v.CanSync(), v.OpenFetch)
		v.toolbarAction(c, OpPull, lucide.ArrowDownToLine, v.text("pull"), size, tr.Behind, !v.CanPull(), v.OpenPull)
		v.toolbarAction(c, OpPush, lucide.ArrowUpFromLine, v.text("push"), size, tr.Ahead, !v.CanSync(), v.OpenPush)
		v.toolbarAction(c, OpStash, lucide.Archive, v.text("stash"), size, 0, !v.CanStash(), v.OpenStash)
	})
}

// toolbarAction builds the toolbar button for op in a keyed container, so its
// state stays with the action as the buttons around it change, and runs action
// when the enabled button is clicked.
func (v *View) toolbarAction(c *ui.Context, op string, icon func() *ui.SVG, label string, size float32, count int, disabled bool, action func()) {
	ui.Box(c).Key("toolbar-" + op).Children(func() {
		onClick(v.toolButton(c, op, icon, label, size, count, disabled), action)
	})
}

// toolButton is a toolbar button for op showing its icon and label. A running
// action shows a spinner in place of the icon, and count, when set, follows
// the label as the commits it moves.
func (v *View) toolButton(c *ui.Context, op string, icon func() *ui.SVG, label string, size float32, count int, disabled bool) ui.Element {
	t := c.Theme()
	b := components.Button(c, "", components.ButtonToolbar).Label(label).Height(size).
		Padding(0, t.Space(3)).Shrink(0).Gap(t.Space(1.5)).AlignItems(ui.Center)
	if disabled {
		b.Disabled(true)
	}
	b.Children(func() {
		if v.Syncing() == op {
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		} else {
			components.Icon(c, icon).Size(16, 16).Shrink(0)
		}
		components.Text(c, label).SingleLine()
		if count > 0 {
			components.Text(c, strconv.Itoa(count)).TextColor(t.TextMuted).SingleLine()
		}
	})
	if tip := v.ActionTooltip(op); tip != "" {
		components.Tooltip(c, b, tip)
	}
	return b
}

// onClick runs action when the enabled button b is clicked.
func onClick(b ui.Element, action func()) {
	if !b.IsDisabled() && b.Clicked() {
		action()
	}
}
