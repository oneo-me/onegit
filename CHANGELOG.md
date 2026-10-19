# Changelog

User-facing release notes for Onegit.

## [Unreleased]

- Browse many Git repositories in one native window; it reopens with the same size and sidebar layout.
- Choose the project parent directory, theme and language in Settings, saved under `~/.onegit/` with legacy settings imported.
- Scan the parent directory in the background into a project tree that marks missing paths and remembers expansion.
- Pin projects to a flat, name-sorted group; both sidebar groups collapse independently.
- Find projects and open Settings from the command palette (`⌘E`).
- Switch between a project's worktrees with tabs in the title bar when it has several, returning to the last project and worktree.
- Fetch, pull, push and stash from the title bar, with dialogs and commit counts; fetch every remote or one branch.
- Push a repository without remotes to a new GitHub repository, private unless you choose public, using the GitHub CLI.
- Browse changes, commits, filterable branches, remote branches, tags and stashes, open their commits, delete refs and see stashed files.
- Double-click a ref or commit to check it out, keeping, stashing or discarding changes; a branch checked out elsewhere opens its worktree.
- Apply a stash by double-clicking it, deleting it afterwards unless you keep it or it conflicts.
- Rebase the checked-out branch from a dialog that names both sides, tests for conflicts, and can update dependent branches or stash local changes.
- Act on refs and commits from their menus: checkout, worktree, pull, merge, rebase, branch, tag, reset, cherry-pick, revert, patch, delete or copy.
- Branch menus also push, fast-forward, rename, change tracking and delete a branch with its worktree.
- Fetch, edit or delete remotes from their menu, fetch one automatically when a worktree opens, view a GitHub remote or copy its address.
- Create branches and tags from their group menus, and remove the stale local branches you choose.
- Apply, rename or delete a stash from its menu, or save its patch.
- Split changes into unstaged and staged lists, stage, unstage, discard or stash chosen files, copy paths, and stage changes or selected lines.
- Commit the staged files with a message (`⌘Enter`).
- See the full commit graph with collapsible merges, find, avatars, and commit details with diffs and file contents.
- Install Onegit on macOS, Windows and Linux from a disk image, an installer, a Debian package or an archive.
- Avoid the UI freezing while choosing the project parent directory.
