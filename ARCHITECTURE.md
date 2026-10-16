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
| `source/git_reader.go`, `source/load.go` | Background Git reads and stale-result guarding |
| `source/repository_*.go` | Toolbar, project sidebar, changes, commit graph and commit details |
| `source/assets/` | App icon: an Icon Composer project for macOS, PNG and ICO for others |
| `source/mygo.json` | Packaging; output goes to `dist/` |

## Boundaries

- The worktree is the unit of context. Branch and working tree state belong to a worktree, never to the repository as a whole.
- Selecting a worktree changes what Onegit reads. It never runs `git checkout` or `git switch`.
- Git write commands run only from an explicit user action: fetch, pull, push, stash and branch switch.
- Status is read when a project is entered and after a user action. Onegit does not poll or watch files.
- Git calls run off the main thread with a clean Git environment. Results return to the main thread and are dropped if the project or worktree changed meanwhile.
- The sidebar tree mirrors the disk. It does not infer links between a repository and worktrees outside the parent directory.
- Missing projects are marked, never deleted. Only a confirmed missing path counts; permission or Git errors do not.
- Projects switch through the sidebar only. Tabs are reserved for worktrees inside one project.
- Empty states stay blank, with no welcome or placeholder text. Errors are always shown.

## Core flows

**Startup.** Onegit restores the cached tree for the selected parent directory, then scans it in the background. Scan results merge into the cache by path and keep the user's expansion changes. See [Project discovery](docs/architecture/project-discovery.md).

**Opening a project.** Selecting a repository lists all its worktrees, including those outside the parent directory. Onegit then reads changes, commits and refs for the selected worktree. See [Repository page](docs/architecture/repository-page.md).

**User Git actions.** A toolbar action runs one Git command in the background, shows failures in the toolbar and reloads repository data when it finishes.

## Data and state

Configuration lives in `~/.oneo/onegit/config.yaml` and UI state in `~/.oneo/onegit/state.yaml`. The same directory is the MyGo user data directory.

- Both files are written to a temporary file in the same directory, synced, then renamed into place.
- In-memory values change only after a successful write. Load and save errors appear in the UI.
- A missing `config.yaml` is imported once from the legacy `Onegit/settings.json` in the user config directory, which is left in place.
- State covers the project cache per parent directory, expansion, pinning, group collapse and the last selections.
- Worktree lists and Git data are never persisted.

## Verification

`go test` covers settings and persistence, project detection, scan cancellation and cache merging, legacy import, worktree tabs, Git reads, commit graph layout and repository views. Tests use real temporary Git repositories.

Review these manually: the native directory picker, packaging and the app icon on macOS.

## Subjects

| Document | When to read |
| --- | --- |
| [Project discovery](docs/architecture/project-discovery.md) | Changing scanning, the sidebar tree, pinning or the project cache |
| [Repository page](docs/architecture/repository-page.md) | Changing worktree tabs, the toolbar, the project sidebar or commit views |
