package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// projectNode retains only repositories and the folders leading to them.
// Paths, rather than display names, identify nodes.
type projectNode struct {
	Path       string `yaml:"path"`
	Repository bool   `yaml:"repository,omitempty"`
	Bare       bool   `yaml:"bare,omitempty"`
	Open       bool   `yaml:"open"`
	Missing    bool   `yaml:"missing,omitempty"`
	Pinned     bool   `yaml:"pinned,omitempty"`
	Removed    bool   `yaml:"removed,omitempty"`
	savedOpen  bool
	Children   []*projectNode `yaml:"children,omitempty"`
}

func scanProjects(ctx context.Context, root string) (*projectNode, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: not a directory", root)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	// A launcher or development shell may set Git overrides. Discovery must
	// describe each visited directory, not the launcher's repository.
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var firstError error
	record := func(err error) {
		if firstError == nil {
			firstError = err
		}
	}
	var visit func(string) *projectNode
	visit = func(path string) *projectNode {
		if ctx.Err() != nil {
			record(ctx.Err())
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			record(fmt.Errorf("%s: %w", path, err))
			return nil
		}
		marker, head, objects := false, false, false
		for _, entry := range entries {
			switch entry.Name() {
			case ".git":
				marker = true
			case "HEAD":
				head = true
			case "objects":
				objects = entry.IsDir()
			}
		}
		node := &projectNode{Path: path}
		if marker || (head && objects) {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			// --absolute-git-dir verifies repository metadata, including gitfiles
			// used by linked worktrees and initialized submodules.
			cmd := exec.CommandContext(probeCtx, git, "-C", path, "rev-parse", "--absolute-git-dir")
			cmd.Env = env
			_, probeErr := cmd.Output()
			if probeErr == nil {
				arg := "--show-toplevel"
				if !marker {
					arg = "--absolute-git-dir"
				}
				cmd = exec.CommandContext(probeCtx, git, "-C", path, "rev-parse", arg)
				cmd.Env = env
				output, err := cmd.Output()
				if err == nil {
					resolved, err := filepath.EvalSymlinks(strings.TrimSuffix(string(output), "\n"))
					node.Repository = err == nil && resolved == path
					node.Bare = node.Repository && !marker
				}
			}
			if probeCtx.Err() != nil {
				record(fmt.Errorf("%s: %w", path, probeCtx.Err()))
			}
			cancel()
		}
		// A verified repository is a leaf; do not scan its working tree.
		if !node.Repository {
			for _, entry := range entries {
				if entry.Name() == ".git" || !entry.IsDir() {
					continue
				}
				if child := visit(filepath.Join(path, entry.Name())); child != nil {
					node.Children = append(node.Children, child)
				}
			}
		}
		if !node.Repository && len(node.Children) == 0 {
			return nil
		}
		return node
	}
	tree := visit(root)
	return tree, firstError
}
