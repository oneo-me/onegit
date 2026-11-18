package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestDirectoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Onegit", "settings.json")
	if got, err := loadPreferences(path); err != nil || got.ProjectParentDir != "" {
		t.Fatalf("first launch: %+v, %v", got, err)
	}
	s := &settings{path: path}
	dir := filepath.Join(t.TempDir(), "项目 with spaces")
	s.setDirectory(dir)
	got, err := loadPreferences(path)
	if err != nil || got.ProjectParentDir != dir || s.problem != "" {
		t.Fatalf("restore: %+v, %v, %s", got, err, s.problem)
	}
	s.setDirectory("")
	got, err = loadPreferences(path)
	if err != nil || got.ProjectParentDir != "" {
		t.Fatalf("clear: %+v, %v", got, err)
	}
}

func TestSaveFailurePreservesSelection(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "file")
	if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &settings{path: filepath.Join(blocked, "settings.json"), values: preferences{ProjectParentDir: root}}
	s.setDirectory("/new")
	if s.values.ProjectParentDir != root || s.problem == "" {
		t.Fatal("failed save changed selection or hid failure")
	}
	data, err := os.ReadFile(blocked)
	if err != nil || string(data) != "keep" {
		t.Fatal("failed save damaged existing file")
	}
}

// Launcher choices keep known IDs of their kind and reset the rest.
func TestLauncherPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("terminal: zed\neditor: cursor\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadPreferences(path); err != nil || got.Terminal != "" || got.Editor != "cursor" {
		t.Fatalf("launchers: %+v, %v", got, err)
	}
}

func TestMalformedPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreferences(path); err == nil {
		t.Fatal("malformed preferences accepted")
	}
}

func TestSettingsInteraction(t *testing.T) {
	s := &settings{path: filepath.Join(t.TempDir(), "settings.json"), systemLanguage: "zh-CN"}
	s.setDirectory("/Users/example/" + strings.Repeat("long-project-directory-", 16))
	app := show(t, s, s.toolbar)
	app.Press("id:open-settings")
	if !s.open {
		t.Fatal("the settings did not open")
	}

	app.WaitFor("text:项目父目录", time.Second)
	app.WaitFor("text:long-project-directory-long", time.Second)
	button, dialog := app.Find("id:choose-directory").Bounds(), app.Find("id:settings").Bounds()
	if button.Origin.X < dialog.Origin.X || button.Origin.X+button.Size.Width > dialog.Origin.X+dialog.Size.Width {
		t.Fatalf("directory button outside the settings: %+v / %+v", button, dialog)
	}
	app.Press("label:关闭设置")
	if s.open {
		t.Fatal("close did not dismiss settings")
	}
	app.Press("id:open-settings")
	app.Key("Escape")
	if s.open {
		t.Fatal("Escape did not dismiss settings")
	}
}

func TestAppearanceAndLanguage(t *testing.T) {
	s := &settings{path: filepath.Join(t.TempDir(), "settings.json"), systemLanguage: "zh-CN"}
	s.setDirectory("/Users/example/Projects")
	app := show(t, s, s.toolbar)
	s.show("appearance", "")
	app.Settle()
	app.Press("id:theme-select")
	app.Press("id:theme-select-2")
	if s.theme() != "dark" {
		t.Fatal("theme did not change")
	}
	s.show("general", "")
	app.Settle()
	app.Press("id:language")
	app.Press("id:language-2")
	app.WaitFor("text:Project parent directory", time.Second)
	restored, err := loadPreferences(s.path)
	if err != nil || restored.Language != "en" || restored.Theme != "dark" || restored.ProjectParentDir != "/Users/example/Projects" {
		t.Fatalf("preferences not retained: %+v, %v", restored, err)
	}
	s.open = false
	s.invalidate()
	app.Settle()
	app.Press("id:theme")
	app.Press("id:theme-light")
	restored, err = loadPreferences(s.path)
	if err != nil || restored.Theme != "light" {
		t.Fatal("toolbar theme not saved")
	}
	s.show("appearance", "")
	app.WaitFor("id:theme-select >> text:Light", time.Second)
	s.setDirectory("/new/path")
	if s.values.Language != "en" || s.theme() != "light" {
		t.Fatal("directory change reset other preferences")
	}
}

func TestSystemLanguage(t *testing.T) {
	for _, locale := range []string{"zh-CN", "zh_Hans_SG", "en-US", "fr-FR"} {
		s := &settings{systemLanguage: locale}
		want := "Settings"
		if strings.HasPrefix(locale, "zh") {
			want = "设置"
		}
		if s.text("设置") != want {
			t.Fatalf("incorrect system language for %s", locale)
		}
	}
}

func TestDirectorySelectionDoesNotBlock(t *testing.T) {
	s := &settings{open: true, path: filepath.Join(t.TempDir(), "settings.json"), systemLanguage: "zh-CN"}
	selected := t.TempDir()
	var asked []core.PickDirectoriesOptions
	var answer func(string, error)
	s.pickDirectory = func(o core.PickDirectoriesOptions, done func(string, error)) {
		asked = append(asked, o)
		answer = done
	}
	app := show(t, s, s.toolbar)
	app.Press("id:choose-directory")
	if len(asked) != 1 || asked[0].Multiple || !s.choosingDirectory {
		t.Fatalf("asked %+v", asked)
	}
	// The button waits for the picker, which never blocks the window.
	if app.Enabled("id:choose-directory") {
		t.Fatal("a second picker may open")
	}
	app.WaitFor("text:项目父目录", time.Second)
	// Other preferences may change before the native picker returns.
	s.setLanguage("en")
	answer(selected, nil)
	if s.choosingDirectory || s.values.ProjectParentDir != selected || s.values.Language != "en" {
		t.Fatal("incorrect completion state")
	}
	got, err := loadPreferences(s.path)
	if err != nil || got != s.values {
		t.Fatalf("selection not persisted: %+v, %v", got, err)
	}
}

func TestDirectorySelectionResults(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		path      string
		err       error
		wantError bool
	}{
		{name: "cancel"},
		{name: "dialog error", err: errors.New("picker failed"), wantError: true},
		{name: "missing directory", path: filepath.Join(root, "missing"), wantError: true},
		{name: "file", path: file, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &settings{path: filepath.Join(t.TempDir(), "settings.json"), values: preferences{ProjectParentDir: root}}
			s.applyDirectorySelection(tc.path, tc.err)
			if s.values.ProjectParentDir != root || (s.problem != "") != tc.wantError {
				t.Fatalf("unexpected result: %+v", s)
			}
		})
	}
}
