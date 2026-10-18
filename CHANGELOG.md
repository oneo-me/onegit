# Changelog

User-facing release notes for Onegit.

## [Unreleased]

- Browse many Git repositories in one native window with Onevra's default colors, a collapsible, resizable sidebar and its own app icon; it reopens where it was, with the same size and sidebar layout.
- Choose the project parent directory, theme and language in Settings, saved under `~/.onegit/`; an earlier `~/.oneo/onegit/` directory moves there and legacy settings are imported.
- Scan the parent directory in the background into a project tree that marks missing paths and remembers expansion.
- Pin projects to a flat, name-sorted group; both sidebar groups collapse independently.
- Find projects and open Settings from the command palette (`⌘E`).
- Switch between a project's worktrees with tabs in the title bar, returning to the last project and worktree.
- Fetch, pull, push, stash and switch branches from the title bar, with fetch, pull, push and stash dialogs and commit counts; fetch updates every remote or one remote branch.
- Browse current changes, all commits and filterable branches, remote branches, tags and stashes in a project sidebar, jumping to their commits, deleting refs from their menu or seeing the files a stash saved.
- Check out a branch, remote branch, tag or commit by double-clicking it, keeping, stashing or discarding local changes.
- Apply a stash by double-clicking it, deleting it afterwards unless you keep it or it conflicts.
- From a ref's or commit's menu, check it out here or as a new worktree, pull, merge or rebase onto it, branch or tag from it, delete it, along with the worktree that has a branch checked out, or copy its name.
- Split current changes into unstaged files above staged ones, stage, unstage or discard chosen files from their menu or shortcuts, stage single changes or selected lines from the diff, and commit the staged ones with a message (`⌘Enter`).
- See all commits in a colored graph across all refs, with collapsible merges, find, author avatars, and floating commit details with diffs, image previews and file contents.
- Install Onegit on macOS, Windows and Linux from a disk image, an installer, a Debian package or an archive.
- Avoid the UI freezing while choosing the project parent directory.
