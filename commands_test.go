package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func TestCommandSearchAndNavigation(t *testing.T) {
	repo := &projectNode{Path: "/projects/team/Onegit", Repository: true}
	missing := &projectNode{Path: "/projects/old/Onegit", Repository: true, Missing: true}
	s := &settings{systemLanguage: "zh-CN", path: filepath.Join(t.TempDir(), "config.yaml"), projectTree: &projectNode{Path: "/projects", Children: []*projectNode{repo, missing}}}
	var items []base.CommandItem
	for _, c := range s.commandItems() {
		items = append(items, c.item)
	}
	if matches := base.FilterCommands(items, "TEAM onegit"); len(matches) != 1 || matches[0].ID != "project/"+repo.Path {
		t.Fatalf("path search: %+v", matches)
	}
	app := show(t, s, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(240)}, Children: []core.Node{s.worktreeSidebar(cx)}}
	})
	app.Press("id:command-trigger")
	if !s.commands.open {
		t.Fatal("the trigger did not open the palette")
	}
	app.Type("id:command-palette >> role:textbox", "team Onegit")
	app.Key("enter")
	if s.commands.open || s.selectedProject != repo {
		t.Fatal("project command did not select original node")
	}
	app.Advance(time.Second)
	app.Key("mod+f")
	if !s.commands.open {
		t.Fatal("the shortcut did not open the palette")
	}
	app.Type("id:command-palette >> role:textbox", "nothing-matches")
	app.WaitFor("text:没有匹配的项目或设置", time.Second)
	app.Key("enter")
	if !s.commands.open {
		t.Fatal("empty search activated")
	}
	// Escape clears the query first, then dismisses the palette.
	app.Key("escape")
	app.Key("escape")
	if s.commands.open {
		t.Fatal("escape did not dismiss")
	}
	app.Advance(time.Second)
	app.Key("mod+f")
	app.Type("id:command-palette >> role:textbox", "theme")
	app.Key("enter")
	if !s.open || s.commands.open || s.category.GetUntracked() != "appearance" {
		t.Fatal("settings handoff failed")
	}
	app.Advance(time.Second)
	app.WaitFor("label:主题偏好", time.Second)
	app.Key("mod+f")
	if s.commands.open {
		t.Fatal("shortcut escaped settings modal")
	}
	app.Press("id:theme-select")
	app.Press("id:theme-select-2")
	saved, err := loadPreferences(s.path)
	if err != nil || saved.Theme != "dark" {
		t.Fatalf("command settings did not persist: %+v %v", saved, err)
	}
}

func TestCommandDismissalAndSettingsDestinations(t *testing.T) {
	s := &settings{systemLanguage: "en-US"}
	app := show(t, s, s.commandTrigger)
	for _, tc := range []struct{ query, shows string }{{"language", "label:Language preference"}, {"directory", "text:Choose folder…"}, {"settings", "text:Project parent directory"}} {
		app.Advance(time.Second)
		app.Key("mod+f")
		app.Type("id:command-palette >> role:textbox", tc.query)
		app.Key("enter")
		if !s.open {
			t.Fatalf("%s did not open the settings", tc.query)
		}
		app.Advance(time.Second)
		app.WaitFor(tc.shows, time.Second)
		app.Key("escape")
		if s.open {
			t.Fatalf("%s: escape did not close the settings", tc.query)
		}
	}
}
