# Repository page

The repository page shows one selected project. The window's title bar holds the project's name or its worktree tabs, then the view's toolbar, as in Neorio, rather than a strip of their own. A hairline separates it from the [Git view](git-view.md) of the selected worktree. Onegit owns the title bar's layout and tabs; the view owns its toolbar and everything below it.

## Worktree tabs

The segmented tabs list every worktree Git reports for the repository, including worktrees outside the parent directory. The main worktree comes first, as Git lists it, named after its directory: the project's name. Tabs appear only for more than one worktree; otherwise, and while loading, the project's name stands in their place. Many tabs scroll before the actions.

Switching tabs only changes the path the view shows. A worktree added from a ref's menu gets its tab once Git lists it, one deleted with its branch loses it, and the shown tab stays. Double-clicking a branch another worktree has checked out selects that worktree's tab. Switching projects or worktrees, or closing the window, drops in-flight reads and discards their results.

## Title bar actions

The view's `Toolbar`, shared with Neorio's Git window, follows the name or tabs: raised squares for Fetch, Pull, Push and Stash that rest as their icon and widen on hover or keyboard focus to show the name, then a commit count when there is one. A running action shows a spinner in place of its icon. The view runs one action at a time per worktree, shows failures as toasts and rereads the worktree after each action.

## Onegit's choices for the view

- Watching is off: the view reads when a worktree is shown and after an action.
- No ref filter and no inserted history rows.
- The view's layout, ref group disclosure, line wrapping, drafts, pinned refs, reflog choice, merge type, fetch, pull, push, stash, rebase and cherry-pick options, and remotes fetched automatically are saved in `state.yaml` under `gitView`.
- Commit authors' avatars are cached in `~/.onegit/avatars/`, downloaded from GitHub or Gravatar.

## Selection defaults

The last selected project and the worktree per project are saved in state. Without a valid saved choice, Onegit selects:

- the first project that is not missing, in sidebar order with pinned projects first, ignoring collapse;
- the first worktree Git lists.

The view shows the worktree's changes when it has some, else all commits. A missing saved project falls back to an available one. With nothing to select, the content area stays blank.
