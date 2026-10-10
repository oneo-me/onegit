package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
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
	for _, width := range []int{820, 1100} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			s := &settings{path: filepath.Join(t.TempDir(), "settings.json"), systemLanguage: "zh-CN"}
			s.setDirectory("/Users/example/" + strings.Repeat("long-project-directory-", 16))
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Children(func() { s.toolbar(c); s.welcome(c); s.render(c) })
			}, width, 720)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			if err := tt.Click("设置"); err != nil {
				t.Fatal(err)
			}
			if !s.open || !tt.HasText("项目父目录") || !tt.HasText(s.values.ProjectParentDir) {
				t.Fatal("settings missing", tt.Texts())
			}
			if _, ok := tt.Find("清除"); ok {
				t.Fatal("unexpected extra directory action")
			}
			layout := ui.NewTester(s.render, width, 720)
			layout.SetPreferences(ui.Preferences{ReduceMotion: true})
			layout.Frame()
			panel, ok := layout.Find("设置")
			if !ok {
				t.Fatal("missing dialog")
			}
			button, ok := layout.Find("选择目录…")
			if !ok || button.X < panel.X || button.X+button.W > panel.X+panel.W || button.Y+button.H > panel.Y+panel.H {
				t.Fatalf("directory button outside dialog: %+v / %+v", button, panel)
			}
			if err := tt.Click("关闭设置"); err != nil {
				t.Fatal(err)
			}
			if s.open {
				t.Fatal("close did not dismiss settings")
			}
			if err := tt.Click("设置"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if s.open {
				t.Fatal("Escape did not dismiss settings")
			}
		})
	}
}

func TestAppearanceAndLanguage(t *testing.T) {
	s := &settings{path: filepath.Join(t.TempDir(), "settings.json"), systemLanguage: "zh-CN"}
	s.setDirectory("/Users/example/Projects")
	s.open = true
	tt := ui.NewTester(s.render, 1100, 720)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := tt.Click("外观"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("主题偏好"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	if s.theme() != "dark" {
		t.Fatal("theme did not change")
	}
	if err := tt.Click("通用"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("语言偏好"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("English"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Project parent directory") {
		t.Fatal("language did not apply", tt.Texts())
	}
	restored, err := loadPreferences(s.path)
	if err != nil || restored.Language != "en" || restored.Theme != "dark" || restored.ProjectParentDir != "/Users/example/Projects" {
		t.Fatalf("preferences not retained: %+v, %v", restored, err)
	}
	toolbar := ui.NewTester(func(c *ui.Context) { ui.Row(c).FillWidth().Children(func() { s.toolbar(c) }) }, 240, 400)
	toolbar.SetPreferences(ui.Preferences{ReduceMotion: true})
	if err := toolbar.Click("Theme"); err != nil {
		t.Fatal(err)
	}
	if err := toolbar.ChooseMenuItem("Light"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Appearance"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Light") {
		t.Fatal("settings did not reflect toolbar choice", tt.Texts())
	}
	restored, err = loadPreferences(s.path)
	if err != nil || restored.Theme != "light" {
		t.Fatal("toolbar theme not saved")
	}
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
