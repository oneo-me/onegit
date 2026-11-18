package main

import (
	"fmt"
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/oneo-me/onegit/launchers"
)

// detectLaunchers finds this computer's file manager, terminals and
// development tools off the engine.
func (s *settings) detectLaunchers() {
	post := s.poster()
	go func() {
		found := launchers.Detect()
		post(func() {
			s.installed = found
			s.invalidate()
		})
	}()
}

// launcher is the chosen launcher of kind. The file manager has no
// preference: it is the platform's own.
func (s *settings) launcher(kind string) *launchers.Installed {
	switch kind {
	case launchers.Terminal:
		return launchers.Resolve(s.installed, kind, s.values.Terminal)
	case launchers.Editor:
		return launchers.Resolve(s.installed, kind, s.values.Editor)
	}
	return launchers.Resolve(s.installed, kind, "")
}

// launcherButtons follow the Git actions with raised icon buttons opening the
// selected worktree in the chosen file manager, terminal and development
// tool, then the repository's GitHub page.
func (s *settings) launcherButtons(cx *core.Scope) core.Node {
	key := func() string {
		s.track()
		k := s.overview.SelectedPath + "\x00" + s.gitView().GitHubPage()
		for _, kind := range launchers.Kinds {
			if l := s.launcher(kind); l != nil {
				k += "\x00" + l.ID
			}
		}
		return k
	}
	return region(cx, key, func(cx *core.Scope) core.Node {
		var buttons []core.Node
		if dir := s.overview.SelectedPath; dir != "" {
			for _, kind := range launchers.Kinds {
				if l := s.launcher(kind); l != nil {
					buttons = append(buttons, ui.IconButton{Icon: launchers.Glyph(kind), Label: fmt.Sprintf(s.text("在 %s 中打开"), l.Name),
						Variant: ui.IconButtonRaised, Size: 32, IconSize: 16, TestID: "launcher-" + kind, OnPress: func() { s.openWith(l, dir) }})
				}
			}
		}
		if page := s.gitView().GitHubPage(); page != "" {
			buttons = append(buttons, ui.IconButton{Icon: lucide.Globe, Label: s.text("在 GitHub 上打开"), Variant: ui.IconButtonRaised, Size: 32, IconSize: 16,
				TestID: "open-github", OnPress: func() { cx.OpenURL(page, nil) }})
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: buttons}
	})
}

// openWith opens dir with the launcher off the engine, showing a toast
// when it fails.
func (s *settings) openWith(l *launchers.Installed, dir string) {
	run := s.runLauncher
	if run == nil {
		run = launchers.Run
	}
	cmd, name := l.Command(dir), l.Name
	post := s.poster()
	go func() {
		if err := run(cmd); err != nil {
			post(func() {
				s.showToast(ui.ToastContent{Type: ui.ToastError, Title: fmt.Sprintf(s.text("无法打开 %s"), name), Description: err.Error()})
			})
		}
	}()
}

// launcherSetting offers automatic choice, the installed launchers of kind
// and hiding the button. A saved launcher that is not installed stays listed.
// Only it is built again once the launchers are found.
func (s *settings) launcherSetting(kind, label string) core.Node {
	value := func() string {
		s.track()
		if kind == launchers.Terminal {
			return s.values.Terminal
		}
		return s.values.Editor
	}
	key := func() string {
		s.track()
		k := ""
		for _, l := range s.installed {
			k += l.ID + ","
		}
		return k
	}
	return core.Func(func(cx *core.Scope) core.Node {
		return region(cx, key, func(cx *core.Scope) core.Node {
			choices := []string{""}
			for _, l := range s.installed {
				if l.Kind == kind {
					choices = append(choices, l.ID)
				}
			}
			if v := core.Untracked(cx, value); v != "" && v != launchers.None && !slices.Contains(choices, v) {
				choices = append(choices, v)
			}
			choices = append(choices, launchers.None)
			var options []ui.Option[string]
			for _, id := range choices {
				options = append(options, ui.Option[string]{Value: id, Label: s.launcherName(kind, id)})
			}
			return settingsSelect(label, options, value, func(chosen string) {
				next := s.values
				if kind == launchers.Terminal {
					next.Terminal = chosen
				} else {
					next.Editor = chosen
				}
				s.save(next)
			}, "launcher-"+kind)
		})
	})
}

// launcherName labels a choice; automatic names the launcher it resolves to.
func (s *settings) launcherName(kind, id string) string {
	switch id {
	case launchers.None:
		return s.text("不显示")
	case "":
		if l := launchers.Resolve(s.installed, kind, ""); l != nil {
			return fmt.Sprintf(s.text("自动（%s）"), l.Name)
		}
		return s.text("自动")
	}
	if l := launchers.Resolve(s.installed, kind, id); l != nil && l.ID == id {
		return l.Name
	}
	if l, ok := launchers.ByID(id); ok {
		return fmt.Sprintf(s.text("%s（未安装）"), l.Name)
	}
	return id
}
