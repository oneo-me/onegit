package gitview

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// dialogSpec is one of the view's dialogs: its state's open flag, its words,
// the fields its body holds and the actions ending it. Body and actions are
// built as it opens, and their controls follow the view's changes.
type dialogSpec struct {
	open        *bool
	title       func() string
	description func() string
	// alert makes it a confirmation, which asks before something that
	// cannot be undone.
	alert bool
	// width is its width, the dialog's own when 0, and height a fixed
	// height for one holding a list.
	width, height core.Points
	body          func(cx *core.Scope) []core.Node
	actions       func(cx *core.Scope) []core.Node
	// confirm runs on Return, unless it is nil.
	confirm func()
	testID  string
}

// dialogNode shows the dialog while its state is open.
func (v *View) dialogNode(cx *core.Scope, d dialogSpec) core.Node {
	open := func() bool { v.track(); return *d.open }
	shown := core.NewMemo(cx, open)
	built := func(build func(cx *core.Scope) []core.Node) core.Node {
		if build == nil {
			return nil
		}
		return core.Dynamic(func(cx *core.Scope) core.Node {
			if !shown.Get() {
				return nil
			}
			var nodes []core.Node
			cx.Untrack(func() { nodes = build(cx) })
			return core.Group(nodes)
		})
	}
	style := core.Style{}
	if d.width > 0 {
		style.Width, style.MaxWidth = core.Pt(d.width), core.Pct(100)
	}
	if d.height > 0 {
		style.Height = core.Pt(d.height)
	}
	dialog := ui.Dialog{Open: open, TestID: d.testID, Inset: v.opts.DialogInset, Style: style,
		OnOpenChange: func(o bool, _ reason) {
			if !o && *d.open {
				*d.open = false
				v.invalidate()
			}
		},
		TitleFunc: func() string { v.watch(); return d.title() },
		Children:  []core.Node{built(d.body)},
		Footer:    []core.Node{built(d.actions)},
		OnConfirm: d.confirm}
	if d.description != nil {
		dialog.DescriptionFunc = func() string { v.watch(); return d.description() }
	}
	if d.alert {
		dialog.Role = core.RoleAlertDialog
	}
	return dialog
}

// closeDialog closes the dialog of open.
func (v *View) closeDialog(open *bool) func() {
	return func() {
		*open = false
		v.invalidate()
	}
}

// cancelButton closes the dialog of open.
func (v *View) cancelButton(open *bool) core.Node {
	return ui.Button{Text: v.text("cancel"), Variant: ui.ButtonSecondary, TestID: "cancel", OnPress: v.closeDialog(open)}
}

// confirmButton runs press while ready says it can, as a dialog's main
// action.
func (v *View) confirmButton(label string, ready func() bool, press func(), testID string) core.Node {
	return ui.Button{Text: label, Variant: ui.ButtonPrimary, TestID: testID, Disabled: v.is(func() bool { return ready != nil && !ready() }),
		OnPress: func() {
			if ready == nil || ready() {
				press()
			}
		}}
}

// Label widths of the dialogs' fields, in points.
const (
	syncLabelWidth   = 64
	refLabelWidth    = 136
	remoteLabelWidth = 80
)

// formField is a dialog row with its label, width wide, aligned to the
// others' end; a disabled one shows its value faded.
func formField(cx *core.Scope, width core.Points, label string, disabled func() bool, value ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	text := ui.Text(label, 13, ui.Regular, colors.Text)
	text.MaxLines = 1
	text.Style.Width, text.Style.FlexShrink, text.Style.TextAlign = core.Pt(width), 0, core.TextAlignEnd
	values := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		Children: value}
	r := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{text, values}}
	if disabled != nil {
		r.StyleFunc = func() core.Style {
			if disabled() {
				return core.Style{Opacity: core.Alpha(0.5)}
			}
			return core.Style{}
		}
		r.PointerEvents = core.PointerEventsBoxNone
	}
	return r
}

// formOption is a check box aligned with the values of formFields whose
// labels are width wide.
func (v *View) formOption(width core.Points, checked *bool, label string, changed func()) core.Node {
	return core.View{Style: core.Style{Padding: core.Edges{Left: width + 12}}, Children: []core.Node{v.checkbox(checked, label, changed)}}
}

// checkbox edits checked, then runs changed unless it is nil.
func (v *View) checkbox(checked *bool, label string, changed func()) core.Node {
	return ui.Checkbox{Text: label, Checked: func() bool { v.watch(); return *checked }, OnChange: func(on bool) {
		*checked = on
		if changed != nil {
			changed()
		}
		v.edited()
	}}
}

// toggle is a switch editing on, with its label after it.
func (v *View) toggle(cx *core.Scope, on *bool, label string, changed func()) core.Node {
	colors := ui.ColorsOf(cx)
	return row(8, ui.Switch{Label: label, Size: ui.SizeSmall, Checked: func() bool { v.watch(); return *on }, OnChange: func(value bool) {
		*on = value
		if changed != nil {
			changed()
		}
		v.edited()
	}}, ui.Text(label, 13, ui.Regular, colors.Text))
}

// textField edits value, focused as it shows when focus is set; submit,
// unless nil, runs on Return.
func (v *View) textField(value *string, label, placeholder string, focus bool, submit func(), testID string) core.Node {
	field := ui.Input{Value: func() string { v.watch(); return *value }, Label: label, Placeholder: placeholder, AutoFocus: focus, TestID: testID,
		Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
		OnChange: func(s string) { *value = s; v.edited() }}
	if submit != nil {
		field.OnSubmit = func(s string) {
			*value = s
			submit()
		}
	}
	return field
}

// radioChoice is one choice of a radio group.
type radioChoice[T comparable] struct {
	value T
	label string
}

// radios choose value among choices, stacked.
func radios[T comparable](v *View, value *T, label string, choices []radioChoice[T], changed func()) core.Node {
	var nodes []core.Node
	for _, c := range choices {
		nodes = append(nodes, ui.Radio{Text: c.label, Checked: func() bool { v.watch(); return *value == c.value }, OnChange: func(on bool) {
			if on {
				*value = c.value
				if changed != nil {
					changed()
				}
				v.edited()
			}
		}})
	}
	return ui.RadioGroup{Label: label, Children: nodes, Style: core.Style{Gap: 8}}
}

// selectField chooses value among options, running changed after a
// choice unless it is nil.
func selectField[T comparable](v *View, value *T, label string, options []ui.Option[T], changed func(), testID string) core.Node {
	return ui.Select[T]{Options: options, Label: label, TestID: testID, Style: core.Style{Width: core.Pct(100), FlexShrink: 1, MinWidth: core.Pt(0)},
		Value: func() T { v.watch(); return *value },
		OnChange: func(t T) {
			*value = t
			if changed != nil {
				changed()
			}
			v.edited()
			v.invalidate()
		}}
}

// choice is a value shown with its icon, as a dialog shows what it acts
// on.
func choice(cx *core.Scope, icon func() *core.SVG, label string) core.Node {
	colors := ui.ColorsOf(cx)
	return row(8, ui.Icon(icon, 14, colors.Text), line(label, 13, ui.Regular, colors.Text))
}

// branchChoice shows a branch's name.
func branchChoice(cx *core.Scope, name string) core.Node { return choice(cx, lucide.GitBranch, name) }

// dialogNote is a dialog's line of text in the role's color.
func dialogNote(text string, size core.Points, color func() core.Color) core.Node {
	return core.Text{Content: text, Style: core.Style{FontSize: size}, StyleFunc: ui.Colored(color)}
}

// busyNote is a dialog's spinner beside what it waits for.
func busyNote(cx *core.Scope, text string) core.Node {
	return row(8, spinner(14), muted(cx, text, 12))
}
