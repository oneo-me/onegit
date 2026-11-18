package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// desktop is the environment the tests' windows show in.
var desktopEnvironment = nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop})

// show shows part of the window, built by build once s is attached, with
// the window's dialogs and toasts, in a headless desktop window whose
// engine runs on the test's goroutine.
func show(t *testing.T, s *settings, build func(cx *core.Scope) core.Node) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		s.attach(cx)
		return s.localized(cx, func(cx *core.Scope) core.Node {
			return core.Group{build(cx), s.settingsDialog(cx), s.commandPalette(cx), s.groupDialogView(cx), ui.Toasts{Toaster: s.toasts}}
		})
	}, nsgotest.Size(1100, 720), desktopEnvironment)
}

// waitFor settles app until cond holds.
func waitFor(t *testing.T, app *nsgotest.App, cond func() bool) {
	t.Helper()
	app.WaitUntil(10*time.Second, cond)
}

// popupLabels lists the labels of the context menu shown.
func popupLabels(app *nsgotest.App) []string {
	var labels []string
	for _, item := range app.PopupMenu {
		if item.Separator {
			labels = append(labels, "-")
		} else {
			labels = append(labels, item.Label)
		}
	}
	return labels
}
