# Repository page

The repository page shows one selected project. The window's title bar holds its worktree tabs and actions, as in Neorio, rather than a title or a strip of their own. Below it comes the [Git view](git-view.md) of the selected worktree. Onegit owns the title bar; the view owns everything below it.

## Worktree tabs

The segmented tabs list every worktree Git reports for the repository, including worktrees outside the parent directory. The main worktree comes first, as Git lists it, named after its directory: the project's name. The tabs replace the title, so a single worktree, or one still loading, still shows one tab; many tabs scroll before the actions.

Switching tabs only changes the path the view shows. Switching projects or worktrees, or closing the window, drops in-flight reads and discards their results.

## Title bar actions

The title bar's round toolbar buttons, raised like the chosen tab, are only an entry point; every action runs through the view.

- Fetch, Pull and Push open the view's dialogs; Pull and Push show the commits a pull would bring or a push would send.
- Stash and the branch menu run `git stash push --include-untracked` and `git switch` through the view's `Run`.
- The view runs one action at a time, shows failures as toasts and rereads the worktree after each action.

## Onegit's choices for the view

- Watching is off: the view reads when a worktree is shown and after an action.
- No ref filter and no inserted history rows.
- The view's layout, ref group disclosure, line wrapping and fetch, pull and push options are saved in `state.yaml` under `gitView`.
- Commit authors' avatars are cached in `~/.onegit/avatars/`, downloaded from GitHub or Gravatar.

## Selection defaults

The last selected project and the worktree per project are saved in state. Without a valid saved choice, Onegit selects:

- the first project that is not missing, in sidebar order with pinned projects first, ignoring collapse;
- the first worktree Git lists.

The view shows the worktree's changes when it has some, else all commits. A missing saved project falls back to an available one. With nothing to select, the content area stays blank.
