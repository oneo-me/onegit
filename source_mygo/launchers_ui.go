package main

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onegit/source_mygo/launchers"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

// detectLaunchers finds this computer's file manager, terminals and
// development tools off the main thread.
func (s *settings) detectLaunchers(onMain func(func())) {
	go func() {
		found := launchers.Detect()
		onMain(func() {
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
func (s *settings) launcherButtons(c *ui.Context) {
	if dir := s.overview.SelectedPath; dir != "" {
		for _, kind := range launchers.Kinds {
			if l := s.launcher(kind); l != nil {
				ui.Box(c).Key("launcher-" + kind).Children(func() {
					label := fmt.Sprintf(s.text("在 %s 中打开"), l.Name)
					if components.IconButton(c, launchers.Glyph(kind)(), label, components.IconButtonToolbar).Clicked() {
						s.openWith(l, dir)
					}
				})
			}
		}
	}
	if page := s.gitView().GitHubPage(); page != "" {
		if components.IconButton(c, lucide.Globe(), s.text("在 GitHub 上打开"), components.IconButtonToolbar).Clicked() {
			c.OpenURL(page)
		}
	}
}

// openWith opens dir with the launcher off the main thread, showing a toast
// when it fails.
func (s *settings) openWith(l *launchers.Installed, dir string) {
	run := s.runLauncher
	if run == nil {
		run = launchers.Run
	}
	cmd, name, onMain := l.Command(dir), l.Name, s.repositoryDispatch()
	go func() {
		if err := run(cmd); err != nil {
			onMain(func() {
				s.toast = &ui.Toast{Title: fmt.Sprintf(s.text("无法打开 %s"), name), Description: err.Error(), Type: components.ToastError}
				s.invalidate()
			})
		}
	}()
}

// launcherSetting offers automatic choice, the installed launchers of kind
// and hiding the button. A saved launcher that is not installed stays listed.
func (s *settings) launcherSetting(c *ui.Context, value *string, kind, label string) {
	choices := []string{""}
	for _, l := range s.installed {
		if l.Kind == kind {
			choices = append(choices, l.ID)
		}
	}
	if *value != "" && *value != launchers.None && !slices.Contains(choices, *value) {
		choices = append(choices, *value)
	}
	choices = append(choices, launchers.None)
	chosen := *value
	if settingsSelect(c, &chosen, label, choices, func(id string) string { return s.launcherName(kind, id) }).Changed() {
		next := s.values
		if kind == launchers.Terminal {
			next.Terminal = chosen
		} else {
			next.Editor = chosen
		}
		s.save(next)
		c.Invalidate()
	}
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
