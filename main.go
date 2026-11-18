// Command onegit is Onegit: a worktree-centered app for browsing many Git
// repositories. Its window lists the projects of a parent directory beside
// the selected repository's Git view (gitview), keeping its preferences in
// ~/.onegit/config.yaml and its state in state.yaml beside them.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

// The window's size when it first opens, and the smallest it may be.
const (
	windowWidth, windowHeight       = 1100, 720
	minWindowWidth, minWindowHeight = 820, 600
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	configDir := filepath.Join(home, ".onegit")
	if err := migrateDataDir(configDir, filepath.Join(home, ".oneo", "onegit")); err != nil {
		log.Printf("Move data directory: %v", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		log.Fatal(err)
	}
	s := newSettings(configDir)
	options := []nsgo.Option{nsgo.DataDir(configDir), nsgo.WindowSize(windowWidth, windowHeight), nsgo.WindowMaterial(core.WindowMaterialSidebar),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: minWindowWidth, MinHeight: minWindowHeight})}
	if f := s.state.Window.frame(); f != nil {
		options = append(options, nsgo.WindowFrame(*f))
	}
	nsgo.Run(s.root, options...)
	s.close()
}
