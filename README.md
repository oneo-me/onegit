# Onegit

**English** | [简体中文](README.zh-CN.md)

Onegit is a worktree-centered macOS app that gathers many Git repositories in one window. It covers everyday repository browsing and worktree switching, not every feature of a general Git client.

## Features

- Scan a parent directory for Git repositories and browse them in a sidebar tree.
- Pin projects to a flat, name-sorted group above the directory tree.
- Find projects and settings with the command palette (`⌘E`).
- Switch between all worktrees of a project with tabs.
- Review staged and unstaged changes, stage whole files, single changes or selected lines, stash chosen files or copy their paths, and commit them with a message.
- Browse the full commit graph with collapsible merges, find in history, and commit details with diffs and file contents.
- Open or reveal a changed or committed file, save the version shown, or reset a file to a commit's or its parent's version from its menu.
- Browse local branches, remote branches and tags, create branches or tags from their group menus, delete stale local branches, and jump to their commits.
- Double-click a ref or commit to check it out, or use its menu for checkout, worktrees, pull, merge, rebase, branch, tag, delete and copy; branch menus also push, fast-forward, rename and track.
- Manage remotes from their menu: fetch, edit or delete one, fetch it when a worktree opens, open it on GitHub or copy its address.
- Rebase from a dialog that reports conflicts and can update dependent branches or stash local changes.
- Reset, cherry-pick, revert or save a commit as a patch, and apply, rename or delete a stash, from their menus.
- Every Git write, such as fetch, pull, push, stash or a menu action, runs only when you click, except remotes you set to fetch automatically when a worktree opens.

## Run

Onegit is not released yet. Build it from source as described in [Development](#development). Git must be on `PATH` at runtime.

## Getting started

1. Open Settings with `⌘,` and choose the parent directory that holds your projects.
2. Wait for the background scan to add repositories to the sidebar.
3. Select a project, then pick a worktree tab.
4. Choose Current changes or All commits in the project sidebar.

## Development

Requirements: macOS, Go 1.27.1 or later, and Xcode Command Line Tools. The MyGo CLI is a Go tool dependency in [go.mod](source/go.mod), so no global install is needed.

Run from `source/`:

```sh
cd source
GOWORK=off go mod download
go tool mygo dev
```

`go tool mygo dev` runs `Onegit Dev` and restarts it after code changes.

Format, test, vet and build from `source/`:

```sh
gofmt -l .
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go tool mygo vet
GOWORK=off go tool mygo build
```

Tests are headless and never open windows or native dialogs. The directory picker, packaging and the app icon need manual review. Builds go to `dist/<os>-<arch>/` with ad-hoc signing and no notarization.

Build release packages for every platform from `source/`: a universal macOS app and disk image, Windows installers and Linux archives and Debian packages. Windows installers need NSIS (`brew install makensis`).

```sh
GOWORK=off go tool mygo build -platform darwin/universal,windows/amd64,windows/arm64,linux/amd64,linux/arm64
```

Neorio users get the same Run, Build, Test and Vet tasks from [.neorio/config.yaml](.neorio/config.yaml); Build makes these release packages.

Read [ARCHITECTURE.md](ARCHITECTURE.md) before changing code, and [AGENTS.md](AGENTS.md) for working rules. User-visible changes are listed in [CHANGELOG.md](CHANGELOG.md).
