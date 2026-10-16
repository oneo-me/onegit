# Git view

`source/gitview` (import path `github.com/oneo-me/onegit/source/gitview`) is a reusable repository view, ported from Neorio's Git window so both apps can share it. Hosts own the window, the project list and the toolbar. The view owns the refs pane, the content beside it and its dialogs.

## What it shows

- **Refs pane.** Current changes, shown only while the worktree has uncommitted changes. All commits with its count. Branches, remote branches and tags as folder trees, with a filter at the bottom. Choosing a ref selects its commit. A ref's context menu deletes it after a confirmation, optionally on its remote too.
- **Changes.** A file tree beside the selected file's diff. Images are compared side by side.
- **All commits.** A graph of every shown ref, the checked out commit and host entries. Merges collapse, `⌘F` finds, and authors show GitHub or Gravatar avatars. Selecting a row floats its details over the list: Commit, Changes and Files tabs. The details can be resized and close with Escape.
- **Dialogs.** Pull and push dialogs, and ref deletion.

## Host API

- `New(Options)` creates a view. `SetPath` chooses the worktree, `Reload` rereads it, `Layout` builds it and `Close` stops it.
- Toolbar entries: `Fetch`, `OpenPull`, `OpenPush` and `Run` for host-defined Git writes. `CanSync` and `CanPull` say whether an action may start. `Syncing` names the running action. `Tracking` and `ActionTooltip` describe the branch and its upstream. `LocalBranches` lists the shown local branches.
- `Selected` names the selected history row. `OnSelect` reports changes to the section or row.

Options:

| Option | Purpose |
| --- | --- |
| `Dispatch` | Required. Runs results on the UI thread |
| `Git` | Environment for the `git` executable |
| `RefFilter` | Hides refs from the pane and the history, with the commits only they reach |
| `Entries` | Rows the host inserts above a revision. Each row has its own columns and file readers |
| `Busy` | Disables Git writes while host work runs |
| `Watch` | Rereads on file changes via MyGo `WatchFiles`; off reads only when asked |
| `State`, `OnState` | Remembered layout and options for the host to persist |
| `Language`, `Text` | Built-in English and Chinese strings. `Text` overrides them by key |
| `AvatarDir` | Avatar cache; empty draws initials and never downloads |
| `Overlay`, `DialogInset` | Host content over the section, and title bar clearance for dialogs |

## Boundaries

- Frame builders do no I/O. Git runs in goroutines, and results carry a generation so stale ones are dropped.
- The view writes only from user actions: fetch, pull, push, ref deletion and the host's `Run`. It never checks out on its own.
- Network commands run with `GIT_TERMINAL_PROMPT=0`, so missing credentials fail instead of hanging.

## Neorio mapping

Neorio can replace its Git window's content with this view:

- `RefFilter` hides `neorio/*` branches.
- `Entries` adds a row per task worktree, with its agents and working time.
- `Busy` reports landings, and `Overlay` shows the command panel.
- `Watch` stays on.

The project sidebar and title bar stay in Neorio. Neorio is not switched over yet.
