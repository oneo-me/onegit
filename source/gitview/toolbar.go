package gitview

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// Toolbar builds the view's Git actions for a host's title bar as round,
// raised buttons of size: fetch, and the pull and push dialogs with the
// commits a pull would bring and a push would send, a stash and the branch
// menu. Every action runs through the view.
func (v *View) Toolbar(c *ui.Context, size float32) {
	tr := v.tracking
	ui.Row(c).Key("gitview-toolbar").Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
		onClick(v.toolButton(c, OpFetch, lucide.Download, v.text("fetch"), size, 0, !v.CanSync()), v.OpenFetch)
		onClick(v.toolButton(c, OpPull, lucide.ArrowDownToLine, v.text("pull"), size, tr.Behind, !v.CanPull()), v.OpenPull)
		onClick(v.toolButton(c, OpPush, lucide.ArrowUpFromLine, v.text("push"), size, tr.Ahead, !v.CanSync()), v.OpenPush)
		onClick(v.toolButton(c, OpStash, lucide.Archive, v.text("stash"), size, 0, !v.CanSync()), func() {
			v.Run(OpStash, v.text("stash_failed"), func(git Git, path string) error {
				_, err := git.Run(path, "stash", "push", "--include-untracked")
				return err
			})
		})
		if b := v.toolButton(c, OpSwitch, lucide.GitBranch, v.text("branch_actions"), size, 0, !v.CanSync()); !b.IsDisabled() {
			b.Menu(v.branchMenu)
		}
	})
}

// branchMenu switches the worktree to another shown local branch.
func (v *View) branchMenu(m *ui.Menu) {
	current := v.tracking.Branch
	branches := v.LocalBranches()
	for _, branch := range branches {
		if m.Item(branch).Checked(branch == current).Chosen() && branch != current {
			v.Run(OpSwitch, v.text("switch_failed"), func(git Git, path string) error {
				_, err := git.Run(path, "switch", branch)
				return err
			})
		}
	}
	if len(branches) == 0 {
		m.Item(v.text("no_branches")).Disabled(true)
	}
}

// toolButton is a toolbar button for op. A running action shows a spinner,
// and count, when set, the commits it moves.
func (v *View) toolButton(c *ui.Context, op string, icon func() *ui.SVG, label string, size float32, count int, disabled bool) ui.Element {
	t := c.Theme()
	b := components.Button(c, "", components.ButtonToolbar).Key("toolbar-"+op).Label(label).Size(size, size).
		Padding(0).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
	if disabled {
		b.Disabled(true)
	}
	b.Children(func() {
		if v.syncing == op {
			components.Spinner(c, components.SpinnerRing).Size(16, 16)
		} else {
			components.Icon(c, icon).Size(16, 16).Shrink(0)
		}
		if count > 0 {
			ui.Box(c).Absolute().Top(1).Right(1).Height(14).MinWidth(14).Padding(0, 4).Radius(7).Center().
				Background(t.Accent).Children(func() {
				components.Text(c, strconv.Itoa(count)).FontSize(10).Bold().TextColor(t.AccentText).SingleLine()
			})
		}
	})
	tip := v.ActionTooltip(op)
	if tip == "" {
		tip = label
	}
	components.Tooltip(c, b, tip)
	return b
}

// onClick runs action when the enabled button b is clicked.
func onClick(b ui.Element, action func()) {
	if !b.IsDisabled() && b.Clicked() {
		action()
	}
}
