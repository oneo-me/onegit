# Git view

`source/gitview` (import path `github.com/oneo-me/onegit/source/gitview`) is a reusable repository view, ported from Neorio's Git window so both apps can share it. Hosts own the window and the project list. The view owns its toolbar, the refs pane, the content beside it and its dialogs.

## What it shows

- **Refs pane.** Current changes, always offered; the view opens on them once read when there are any, and otherwise on the history. All commits with its count.
- With `Worktrees` set and more than one worktree, the worktrees by directory name follow All commits, the shown one in bold; double-clicking another goes to `OpenWorktree`.
- Branches, remote branches and tags as folder trees, then stashes when there are any, with a filter. Choosing a ref selects its commit; a stash lists the files it saved, untracked ones included.
- Pinned refs come first, and the All commits menu shows lost commits from the reflog.
- The Branches heading creates a branch or selects stale ones; Remote branches adds a remote; Tags creates a tag.
- **Ref, commit, remote and stash menus.** Refs, history rows, remotes and stashes have context menus grouped like Fork's.
- A ref checks out, adds a worktree, fast-forwards, pulls, pushes, merges, rebases (also interactively), branches, tags, tracks, renames, pins, deletes or copies.
- A commit branches, tags, merges or rebases into the checked-out branch, resets, checks out, cherry-picks, reverts, saves its patch, compares with the worktree or another chosen commit, or copies its hash or message.
- A branch, remote branch or tag pointing at a commit leads the commit's menu with that ref's menu, which acts as in the refs pane.
- A remote's menu fetches, edits or deletes it, toggles automatic fetching, opens it on GitHub, adds another remote or copies its address.
- A stash applies, renames, saves its patch or deletes. Items that cannot apply are left out or disabled; writes that need input open a dialog.
- Double-clicking a ref or commit, or Enter on a commit, opens the checkout dialog; a local branch switches at once when there are no changes, and one checked out elsewhere goes to `OpenWorktree`.
- Double-clicking a stash opens one applying it.
- **Changes.** A file tree beside the selected file's diff; images are compared side by side. A conflicted file shows its worktree text with Git's markers.
- A file's menu opens it in its default application or shows it in the file manager, stages, unstages or discards it, stashes the chosen files, opens its history or blame, and copies its paths. A conflicted file can be resolved with ours or theirs. A directory's menu opens its folder in the file manager and acts on the files below it.
- **Staging.** Current changes split into an unstaged list above a staged one, each a tree with multi-selection, listed by `WorktreeStatus`.
- Double-clicking a file, a diff change's button or selected lines stage or unstage; an unstaged change or selected lines can also be discarded.
- Row menus and shortcuts act on the chosen rows: Stage (`⌘S`) and Discard Changes (`⇧⌘D`) above, Unstage (`⇧⌘S`) below; headers act on a whole list.
- A file's menu also offers Stage All or Unstage All.
- **Discarding.** After a confirmation, `Discard` restores tracked files from the index and deletes untracked ones. Discarding part of a file writes the worktree version with `DiscardText`; a Host without `FileWriter` cannot do it.
- **Committing.** A commit box below the lists commits the staged files, amends the checked out commit, or commits and pushes; its message is kept as a draft per worktree. `⌘Enter` commits. Partial staging writes the picked text to the index with `StageText`, never touching the worktree.
- **All commits.** A graph of every shown ref, the checked out commit and host entries, or the reflog when shown.
- Merges collapse, `⌘F` finds, and authors show GitHub or Gravatar avatars. `⇧⌘F` searches the repository's message, author, changed paths or content.
- Selecting a row floats its details over the list: Commit, Changes and Files tabs. The details can be resized and close with Escape.
- A commit's Changes menu also resets a file to the commit's version or its parent's after a confirmation, or shows it in the Files tab.
- Changes and Files open, save, reveal or copy a file, open a folder in the file manager, and open a commit's version or save it from a temporary copy.
- **Toolbar.** Labeled buttons for a host's title bar: Fetch, Pull and Push followed by the commits they move, and Stash. A running action shows a spinner.
- Two chosen commits, or a commit and the worktree, open a comparison section with their files and diffs. File History and Blame dialogs list a file's commits or lines.
- **Operations.** A banner names a stopped merge, rebase, cherry-pick, revert or squash, counts the remaining conflicts, and offers Continue and Abort.
- **Dialogs.** Fetch, pull, push, stash, rename-stash, push-tag, reset, cherry-pick, revert, rebase, interactive rebase, remote and stale-branch dialogs, ref deletion, bulk ref actions and the menus' writes.
- A stash takes an optional message and includes untracked files unless unchecked. Applying one runs `stash apply`, then drops it by hash unless kept or applying failed.
- The rebase dialog names both branches, lists the local branches `--update-refs` moves, tests for conflicts with `git replay --ref-action=print`, and offers autostash. Interactive rebase lists the commits from a base to HEAD, with pick, reword, edit, squash, fixup and drop actions and reordering.
- Pushing from a repository without remotes creates a GitHub repository instead: `gh repo create --source` names it and makes it private or public, adds it as `origin`, then the branch is pushed.
- `gh` is looked up on `PATH`, then in Homebrew's directories, or by a `Host` that is also a `GitHubHost`; without it the dialog explains how to install it and sign in. Other hosts keep the plain notice.
- A merge can fast-forward, create a merge commit, squash its changes or stop before committing.
- Bulk dialogs delete or push several local branches or tags.
- Deleting local branches first reads `UnmergedBranches`, those `branch -d` refuses: commits their upstream lacks, or HEAD lacks without one. Such a branch is marked, and goes with `branch -D` only once the user chooses to lose its commits.
- Fetch updates every remote, or with its switch off one remote branch. A file's version or a commit's or stash's patch is written where the host's `SaveFile` or MyGo's save dialog chooses.

| Menu item | Git |
| --- | --- |
| Checkout, Checkout Commit | A dialog for a local branch, the branch tracking a remote branch, a new tracking branch, or a detached tag or commit; local changes stay, are stashed and reapplied, or are discarded |
| Open Worktree, Checkout as Worktree | A branch another worktree has checked out opens that worktree through `OpenWorktree`; otherwise `worktree add` places it beside the main worktree, with the same choices |
| Fast-Forward to | Local branches with an upstream they only lack commits of: `merge --ff-only` in the worktree that has the branch, else `update-ref` |
| Push to | Local branches: the push dialog, preset to that branch; tags: the push-tag dialog |
| Pull into, Pull | Remote branches: the pull dialog, preset to that branch; the checked-out branch: preset to its upstream |
| Merge into, Rebase on, Rebase to Here, Interactive Rebase | `merge` with a type chosen in the dialog (`--no-ff`, `--squash` or `--no-commit`), the rebase dialog, or interactive rebase, of the checked out branch, optionally `--autostash`; conflicts stop for the banner |
| New Branch, New Tag | A branch, checked out by default, or a tag, annotated when given a message; a group heading starts at the checked out commit |
| Add, Edit, Delete Remote | `remote add`, `remote rename` with `remote set-url`, or `remote remove` from a dialog or after a confirmation |
| Fetch Remote, Fetch Automatically | `fetch --prune` of one remote now, or whenever a worktree opens while checked; the setting is saved per repository |
| View on GitHub, Copy Remote Address | The remote's GitHub page in the browser, for a fetch URL hosted there, or that URL copied |
| Select Stale Local Branches | A dialog of local branches whose last commit is older than 90 days and that no worktree has checked out; deleting them runs `branch -d` |
| Reset to Here | `reset` with a soft, mixed or hard mode chosen in a dialog |
| Cherry-pick, Revert | `cherry-pick` or `revert` of a commit, optionally without a commit and with its parent chosen for a merge |
| Save as Patch | A commit with `format-patch`, or a stash with `stash show -p` |
| Tracking, Rename | Local branches: `branch --set-upstream-to` or `--unset-upstream`, and `branch --move` from a dialog |
| Delete | Refs only, after a confirmation, optionally on the remote too. A branch checked out in another worktree, not the shown or main one, needs that worktree's `worktree remove` too, after the unmerged check |
| Stash | Apply, rename or delete a stash from a dialog, or save chosen files from the changes menu |
| Pin, Unpin | Keeps a ref in the Pinned group, saved with the view's state |
| Resolve Conflict | Takes ours or theirs with `checkout --ours` or `--theirs`, then stages the file |
| File History, Blame | `log --follow` for the file, or `blame --line-porcelain` at the worktree or shown commit |
| Show Lost Commits (Reflog), Search History | `log -g --all` in the history, or `git log` searches by message, author, path or pickaxe content |
| Compare to Working Directory, Compare Selected Commits | `diff` between a commit and the worktree, or between two chosen commits, shown in a comparison section |
| Copy | A ref's name, a commit's full hash or message, or the chosen files' paths |
| Open File, Open Folder, Show in File Manager | A file in its default application, a directory row's folder in the file manager, or either shown in the file manager; a commit's or stash's version opens from a temporary copy |
| Save as… | Writes the shown version of a file where the user chooses |
| Show in Files, Reset File to | A commit's Changes shows the file in its Files tab, or resets the index and worktree to the commit's or its parent's version, deleting the file the revision lacks, after a confirmation |
| Continue, Abort | `commit --no-edit` for a merge or squash, `rebase --continue`, `cherry-pick --continue`, `revert --continue`, `am --continue`, or the matching abort |

## Host API

- `New(Options)` creates a view. `SetPath` chooses the worktree, `Reload` rereads it, `Layout` builds it and `Close` stops it.
- `Toolbar` builds the Git actions in a host's title bar, and `GitHubPage` names the GitHub page of the tracked or default remote for a host's own button. `OpWorktree` names a write adding or removing a worktree, for `OnWrite`.
- `OpenFetch`, `OpenPull`, `OpenPush`, `OpenStash` and `Run` serve hosts adding their own. `CanSync`, `CanStage`, `CanPull` and `CanStash` say whether an action may start.
- `Syncing` names the running action. `Tracking` and `ActionTooltip` describe the branch and its upstream. `Fetch` fetches every remote without the dialog.
- `Selected` names the selected history row, and `RevealEntry` selects an entry's row once read. `OnSelect` reports changes to the section or row.

Options:

| Option | Purpose |
| --- | --- |
| `Dispatch` | Required. Runs results on the UI thread |
| `Git` | Environment for the `git` executable, or a `Host` that runs git, reads files and watches them on another computer, and as a `GitHubHost` runs `gh` there |
| `RefFilter` | Hides refs from the pane and the history, with the commits only they reach |
| `Entries` | Rows the host inserts above a revision. Each row has its own leading mark, columns and file readers |
| `Busy` | Disables Git writes while host work runs |
| `OnWrite` | Reports each finished Git write, such as to refresh a host's project list |
| `OpenWorktree` | Shows the worktree of a double-clicked branch another worktree has checked out; nil ignores it |
| `Watch` | Rereads on file changes via MyGo `WatchFiles`, or the host's watch; off reads only when asked |
| `State`, `OnState` | Remembered layout, options, drafts, pinned refs, reflog choice, merge type and automatic fetch choices for the host to persist |
| `Language`, `Text` | Built-in English and Chinese strings. `Text` overrides them by key |
| `AvatarDir` | Avatar cache; empty draws initials and never downloads |
| `SaveFile` | Writes a file's version or a commit's or stash's patch where the host chooses; nil uses MyGo's save dialog |
| `OpenFile`, `RevealFile` | Opens a file in its default application or a directory's folder in the file manager, and shows either in the file manager; nil leaves those menu items out |
| `Overlay`, `DialogInset` | Host content over the section, and title bar clearance for dialogs |

## Boundaries

- Frame builders do no I/O. Git runs in goroutines, and results carry a generation so stale ones are dropped.
- The view writes from user actions: the toolbar, the file, ref, commit and remote menus, staging, discarding, resetting files, commits and the host's `Run`. The only automatic write is fetching a remote the user set to fetch when a worktree opens. It never checks out on its own.
- Network commands run with `GIT_TERMINAL_PROMPT=0`, so missing credentials fail instead of hanging.
- Every action works through a `Host` as it does on this computer. One that runs a program other than git extends `Host` with an optional interface, as `GitHubHost` does for `gh` and `FileWriter` does for discarding part of a file. Interactive rebase itself needs the local Git executable, since its todo and message editors are temporary scripts.
- Conflict resolution covers taking ours or theirs and staging the file; it does not include a side-by-side merge editor.

## Neorio mapping

Neorio's Git window shows this view and leads its title bar with `Toolbar`; its project sidebar and the title bar's task launchers and commands stay in Neorio.

- `RefFilter` hides `neorio/*` branches.
- `Entries` adds a row per task worktree, led by a robot animated while it works, with its agents and working time, and `RevealEntry` selects the task the window opens for.
- `Busy` reports landings, `Overlay` shows the command panel, and `OnWrite` rereads the sidebar's status.
- `Watch` stays on.
- `Git.Host` runs git and `gh`, reads files and watches through the server of the shown device, this computer's or an SSH host's.
