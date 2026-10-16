# Repository page

The repository page shows one selected project. From top to bottom it has worktree tabs, a toolbar, and an Onevra `Split` with the project sidebar on the left and content on the right.

## Worktree tabs

The tabs list every worktree Git reports for the repository, including worktrees outside the parent directory. The main worktree comes first, as Git lists it. The tab strip is hidden when there is only one worktree.

Switching tabs only changes the directory Onegit reads. Switching projects or worktrees, or closing the window, cancels in-flight reads and discards their results.

## Toolbar

The toolbar offers fetch, pull, push and stash on the left and a branch menu on the right. Each action runs one Git command in the background. Failures stay visible in the toolbar until the next action, and success reloads the page data.

## Project sidebar

The sidebar shows the repository name, the Changes and All Commits sections, and collapsible trees of local branches, remote branches, tags, stashes and submodules. A filter box at the bottom narrows those trees by name.

Selecting a branch, remote branch, tag or stash selects its row and reveals its commit in All Commits. Submodule entries point to commits in another repository, so they only select the row. The ref selection is not persisted.

## Content

Changes lists the working tree changes of the selected worktree.

All Commits reads the full history of the repository: every local branch, remote branch and tag, not only the current worktree's branch. The graph is laid out from parent links and colored by branch. The details pane has Commit, Changes and File Tree tabs.

## Selection defaults

The last selected project, the worktree per project and the sidebar section per project are saved in state. Without a valid saved choice, Onegit selects:

- the first project that is not missing, in sidebar order with pinned projects first, ignoring collapse;
- the first worktree Git lists;
- the Changes section.

A missing saved project falls back to an available one. With nothing to select, the content area stays blank.
