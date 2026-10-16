# Onegit

**English** | [简体中文](README.zh-CN.md)

Onegit is a worktree-centered macOS app that gathers many Git repositories in one window. It covers everyday repository browsing and worktree switching, not every feature of a general Git client.

## Features

- Scan a parent directory for Git repositories and browse them in a sidebar tree.
- Pin projects to a flat, name-sorted group above the directory tree.
- Find projects and settings with the command palette (`⌘E`).
- Switch between all worktrees of a project with tabs.
- Review working tree changes, the full commit graph and commit details.
- Browse local branches, remote branches, tags, stashes and submodules, and jump to their commits.
- Fetch, pull, push, stash and switch branches only when you click.

## Run

Onegit is not released yet. Build it from source as described in [Development](#development). Git must be on `PATH` at runtime.

## Getting started

1. Open Settings with `⌘,` and choose the parent directory that holds your projects.
2. Wait for the background scan to add repositories to the sidebar.
3. Select a project, then pick a worktree tab.
4. Choose Changes or All Commits in the project sidebar.

## Development

Requirements: macOS, Go 1.27.1 or later, and Xcode Command Line Tools. The MyGo CLI is a Go tool dependency in [go.mod](source/go.mod), so no global install is needed.

Run from `source/`:

```sh
cd source
GOWORK=off go mod download
go tool mygo dev
```

`go tool mygo dev` runs `Onegit Dev` and restarts it after code changes.

Build, test and vet from `source/`:

```sh
go tool mygo build
GOWORK=off go test ./...
GOWORK=off go tool mygo vet
```

Builds go to `dist/<os>-<arch>/` with ad-hoc signing and no notarization. Neorio users get the same Run, Build, Test and Vet tasks from [.neorio/config.yaml](.neorio/config.yaml).

Read [ARCHITECTURE.md](ARCHITECTURE.md) before changing code, and [AGENTS.md](AGENTS.md) for working rules. User-visible changes are listed in [CHANGELOG.md](CHANGELOG.md).
