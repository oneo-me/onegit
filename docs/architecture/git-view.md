# Git view

`source/gitview` (import path `github.com/oneo-me/onegit/source/gitview`) is a reusable repository view, ported from Neorio's Git window so both apps can share it. Hosts own the window and the project list. The view owns its toolbar, the refs pane, the content beside it and its dialogs.

## What it shows

- **Refs pane.** Current changes, always offered; the view opens on them once read when there are any, and otherwise on the history. All commits with its count. Branches, remote branches and tags as folder trees, then stashes when there are any, with a filter at the bottom. Choosing a ref selects its commit, and a stash lists the files it saved, untracked ones included.
- **Ref and commit menus.** Refs in the pane and history rows share one context menu, mapped below and grouped like Fork's. Items that cannot apply to the target, such as checking out a branch another worktree has, are left out; writes that need input confirm in a dialog. Double-clicking a ref or commit, or Enter on a commit, opens the checkout dialog; double-clicking a stash opens one applying it.
- **Changes.** A file tree beside the selected file's diff. Images are compared side by side.
- **Staging.** Current changes split into an unstaged list above a staged one, each a tree with multi-selection, listed by `WorktreeStatus`. Double-clicking a file, a diff change's button or selected lines stage or unstage. Row menus and shortcuts act on the chosen rows: Stage (`⌘S`) and Discard Changes (`⇧⌘D`) above, Unstage (`⇧⌘S`) below; headers act on a whole list.
- **Discarding.** After a confirmation, `Discard` restores tracked files from the index and deletes untracked ones. A commit box below the lists commits the staged files. Partial staging writes the picked text to the index with `StageText`, never touching the worktree.
- **All commits.** A graph of every shown ref, the checked out commit and host entries. Merges collapse, `⌘F` finds, and authors show GitHub or Gravatar avatars. Selecting a row floats its details over the list: Commit, Changes and Files tabs. The details can be resized and close with Escape.
- **Toolbar.** Round buttons for a host's title bar: Fetch, Pull and Push with the commits they move, Stash, and a menu switching to a shown local branch.
- **Dialogs.** Fetch, pull, push and stash dialogs, ref deletion and the menus' writes. A stash takes an optional message and includes untracked files unless unchecked. Applying one runs `stash apply`, then drops it by hash unless kept or applying failed. Fetch updates every remote, or with its switch off one remote branch.

| Menu item | Git |
| --- | --- |
| Checkout | A dialog for a local branch, the one tracking a remote branch, a new branch tracking a remote branch, or a detached tag or commit; local changes stay, are stashed and reapplied, or are discarded, leaving untracked files |
| Checkout as Worktree | `worktree add` beside the main worktree, with the same choices |
| Fast-Forward to | Local branches with an upstream they only lack commits of: `merge --ff-only` in the worktree that has the branch, else `update-ref` |
| Push to | Local branches: the push dialog, preset to that branch |
| Pull into | Remote branches only: the pull dialog, preset to that branch |
| Merge into, Rebase on | `merge --no-edit` or `rebase` of the checked out branch, optionally `--no-ff` or `--autostash`; conflicts stay for the user |
| New Branch, New Tag | A branch, checked out by default, or a tag, annotated when given a message |
| Tracking, Rename | Local branches: `branch --set-upstream-to` or `--unset-upstream`, and `branch --move` from a dialog |
| Delete | Refs only, after a confirmation, optionally on the remote too. A branch checked out in another worktree, not the shown or main one, needs that worktree's `worktree remove` too, after checking that `branch -d` would succeed |
| Copy | The ref's name or the commit's full hash |

## Host API

- `New(Options)` creates a view. `SetPath` chooses the worktree, `Reload` rereads it, `Layout` builds it and `Close` stops it.
- `Toolbar` builds the Git actions in a host's title bar. `OpWorktree` names a write adding or removing a worktree, for `OnWrite`. Their parts serve hosts adding their own: `OpenFetch`, `OpenPull`, `OpenPush`, `OpenStash` and `Run` for host-defined Git writes. `CanSync`, `CanPull` and `CanStash` say whether an action may start. `Syncing` names the running action. `Tracking` and `ActionTooltip` describe the branch and its upstream. `LocalBranches` lists the shown local branches. `Fetch` fetches every remote without the dialog.
- `Selected` names the selected history row, and `RevealEntry` selects an entry's row once read. `OnSelect` reports changes to the section or row.

Options:

| Option | Purpose |
| --- | --- |
| `Dispatch` | Required. Runs results on the UI thread |
| `Git` | Environment for the `git` executable, or a `Host` that runs git, reads files and watches them on another computer |
| `RefFilter` | Hides refs from the pane and the history, with the commits only they reach |
| `Entries` | Rows the host inserts above a revision. Each row has its own leading mark, columns and file readers |
| `Busy` | Disables Git writes while host work runs |
| `OnWrite` | Reports each finished Git write, such as to refresh a host's project list |
| `Watch` | Rereads on file changes via MyGo `WatchFiles`, or the host's watch; off reads only when asked |
| `State`, `OnState` | Remembered layout and options for the host to persist |
| `Language`, `Text` | Built-in English and Chinese strings. `Text` overrides them by key |
| `AvatarDir` | Avatar cache; empty draws initials and never downloads |
| `Overlay`, `DialogInset` | Host content over the section, and title bar clearance for dialogs |

## Boundaries

- Frame builders do no I/O. Git runs in goroutines, and results carry a generation so stale ones are dropped.
- The view writes only from user actions: the toolbar, the ref and commit menus, staging, discarding, commits and the host's `Run`. It never checks out on its own.
- Network commands run with `GIT_TERMINAL_PROMPT=0`, so missing credentials fail instead of hanging.

## Neorio mapping

Neorio's Git window shows this view and leads its title bar with `Toolbar`; its project sidebar and the title bar's task launchers and commands stay in Neorio.

- `RefFilter` hides `neorio/*` branches.
- `Entries` adds a row per task worktree, led by a robot animated while it works, with its agents and working time, and `RevealEntry` selects the task the window opens for.
- `Busy` reports landings, `Overlay` shows the command panel, and `OnWrite` rereads the sidebar's status.
- `Watch` stays on.
- `Git.Host` runs git and reads files through the server of the task's device, this computer's or an SSH host's.
