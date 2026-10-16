# Onegit architecture

Onegit is a single Go module in [source](source/) built on [Onevra](https://github.com/oneo-me/onevra) components and the MyGo runtime. It has three moving parts: project discovery for the sidebar, a repository page for the selected project, and settings with persisted state. All Git access goes through the `git` executable on `PATH`.

Onevra and MyGo versions are pinned in [go.mod](source/go.mod). Onevra provides themed controls and the `shells/sidebar` window shell; Onegit owns application state, persistence and background work.

## Code map

| Path | Responsibility |
| --- | --- |
| `source/main.go` | MyGo startup, window creation and the app menu |
| `source/settings*.go` | Settings dialog, preferences file, theme and language |
| `source/localization.go` | UI strings keyed by their Chinese text, with English translations |
| `source/commands.go` | Command palette data and actions |
| `source/projects*.go`, `source/state.go` | Project scan, sidebar tree, pinning and persisted sidebar state |
| `source/worktrees*.go` | Worktree listing and the worktree tab strip |
| `source/repository_ui.go` | Repository page and the title bar actions that drive the Git view |
| `source/gitview/` | Reusable Git view shared with Neorio: refs pane, changes, commit graph, details, dialogs and Git CLI layer |
| `source/assets/` | App icon: an Icon Composer project for macOS, PNG and ICO for others |
| `source/mygo.json` | Packaging; output goes to `dist/` |

## Boundaries

- The worktree is the unit of context. Branch and working tree state belong to a worktree, never to the repository as a whole.
- Selecting a worktree changes what Onegit reads. It never runs `git checkout` or `git switch`.
- Git write commands run only from an explicit user action: fetch, pull, push, stash, branch switch and ref deletion.
- Status is read when a project is entered and after a user action. Onegit does not poll or watch files.
- Git calls run off the main thread with a clean Git environment. Results return to the main thread and are dropped if the project or worktree changed meanwhile.
- The sidebar tree mirrors the disk. It does not infer links between a repository and worktrees outside the parent directory.
- Missing projects are marked, never deleted. Only a confirmed missing path counts; permission or Git errors do not.
- Projects switch through the sidebar only. Tabs are reserved for worktrees inside one project.
- Empty states stay blank, with no welcome or placeholder text. Errors are always shown.

## Core flows

**Startup.** Onegit restores the cached tree for the selected parent directory, then scans it in the background. Scan results merge into the cache by path and keep the user's expansion changes. See [Project discovery](docs/architecture/project-discovery.md).

**Opening a project.** Selecting a repository lists all its worktrees, including those outside the parent directory. Onegit then reads changes, commits and refs for the selected worktree. See [Repository page](docs/architecture/repository-page.md).

**User Git actions.** A title bar action runs through the Git view, one at a time in the background. Failures show as toasts, and the view rereads the worktree when the action finishes.

## Data and state

Configuration lives in `~/.onegit/config.yaml` and UI state in `~/.onegit/state.yaml`. The same directory is the MyGo user data directory.

- Both files are written to a temporary file in the same directory, synced, then renamed into place.
- In-memory values change only after a successful write. Load and save errors appear in the UI.
- When `~/.onegit` does not exist, an earlier `~/.oneo/onegit` directory is moved there at startup.
- A missing `config.yaml` is imported once from the legacy `Onegit/settings.json` in the user config directory, which is left in place.
- State covers the main window's bounds and maximized or full-screen state, which MyGo saves through a window state store, the sidebar width and collapse, the project cache per parent directory, expansion, pinning, group collapse, the last selections and the Git view's layout and options.
- Commit authors' avatars are downloaded from GitHub or Gravatar and cached in `avatars/` in the same directory.
- Worktree lists and Git data are never persisted.

## Verification

`go test` covers settings and persistence, project detection, scan cancellation and cache merging, legacy import, worktree tabs, and the Git view's reads, graph layout, ref filtering, inserted rows and actions. Tests use real temporary Git repositories.

Review these manually: the native directory picker, packaging and the app icon on macOS.

## Subjects

| Document | When to read |
| --- | --- |
| [Project discovery](docs/architecture/project-discovery.md) | Changing scanning, the sidebar tree, pinning or the project cache |
| [Repository page](docs/architecture/repository-page.md) | Changing worktree tabs, the title bar actions or how Onegit configures the Git view |
| [Git view](docs/architecture/git-view.md) | Changing `source/gitview` or embedding it in another app |
