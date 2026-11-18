package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/oneo-me/onegit/launchers"
)

// The repository's actions live in the title bar beside its name, not in a
// strip of their own.
func TestRepositoryTitleActions(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "demo")
	testGit(t, "init", repo)
	s := &settings{systemLanguage: "zh-CN", selectedProject: &projectNode{Path: repo, Repository: true}}
	app := show(t, s, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Width: core.Pt(800), Height: core.Pt(52), Padding: core.Symmetric(0, 8), AlignItems: core.AlignCenter},
			Children: []core.Node{s.repositoryTitle(cx)}}
	})
	app.WaitFor("text:demo", time.Second)
	for _, op := range []string{"fetch", "pull", "push", "stash"} {
		if b := app.Find("id:toolbar-" + op).Bounds(); b.Origin.Y < 0 || b.Origin.Y+b.Size.Height > 52 {
			t.Fatalf("%s outside the title bar: %+v", op, b)
		}
	}

	// The toolbar sits in the middle, and the name stays with several
	// worktrees, whose tabs belong to the page.
	fetch, stash := app.Find("id:toolbar-fetch").Bounds(), app.Find("id:toolbar-stash").Bounds()
	if mid := (fetch.Origin.X + stash.Origin.X + stash.Size.Width) / 2; mid < 380 || mid > 420 {
		t.Fatalf("toolbar not centered: %v to %v", fetch, stash)
	}
	s.overview.Worktrees = []worktree{{Path: repo}, {Path: filepath.Join(root, "feature")}}
	s.invalidate()
	if !app.Exists("text:demo") || app.Exists("text:feature") {
		t.Fatal("worktrees in the title bar")
	}
}

// The Git actions are followed by the chosen launchers for the selected
// worktree and the repository's GitHub page; a launcher that fails shows a
// toast.
func TestRepositoryTitleLaunchers(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "demo")
	testGit(t, "init", repo)
	testGit(t, "-C", repo, "remote", "add", "origin", "git@github.com:o/demo.git")
	s := &settings{statePath: filepath.Join(t.TempDir(), "state.yaml"), systemLanguage: "en",
		values: preferences{ProjectParentDir: root, Editor: launchers.None}}
	for _, id := range []string{"finder", "ghostty", "vscode"} {
		l, _ := launchers.ByID(id)
		s.installed = append(s.installed, launchers.Installed{Launcher: l, Path: "/Applications/" + l.Name + ".app"})
	}
	ran := make(chan *exec.Cmd, 1)
	s.runLauncher = func(cmd *exec.Cmd) error { ran <- cmd; return errors.New("denied") }
	app := show(t, s, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Width: core.Pt(1000), Height: core.Pt(52), Padding: core.Symmetric(0, 8), AlignItems: core.AlignCenter},
			Children: []core.Node{s.repositoryTitle(cx)}}
	})
	s.scanProjects()
	waitFor(t, app, func() bool {
		return !s.scanning && !s.overview.Loading && !s.repositoryLoading() && s.gitView().GitHubPage() != ""
	})
	stash := app.Find("id:toolbar-stash").Bounds()
	for _, id := range []string{"launcher-files", "launcher-terminal", "open-github"} {
		if b := app.WaitFor("id:"+id, time.Second).Bounds(); b.Origin.X < stash.Origin.X || b.Origin.X+b.Size.Width > 1000 {
			t.Fatalf("%s before the Git actions: %+v", id, b)
		}
	}
	if app.Exists("id:launcher-editor") {
		t.Fatal("hidden editor shown")
	}
	app.Press("id:open-github")
	if opened := app.OpenedURLs; len(opened) != 1 || opened[0] != "https://github.com/o/demo" {
		t.Fatalf("opened: %v", opened)
	}
	app.Press("id:launcher-terminal")
	select {
	case cmd := <-ran:
		if !strings.HasSuffix(cmd.Args[len(cmd.Args)-1], repo) {
			t.Fatalf("launched %v", cmd.Args)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("launcher did not run")
	}
	waitFor(t, app, func() bool { return s.lastToast.Title != "" })
	if s.lastToast.Title != "Could not open Ghostty" || s.lastToast.Description != "denied" {
		t.Fatalf("toast: %+v", s.lastToast)
	}
}
