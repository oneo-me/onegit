# Changelog

User-facing release notes for Onegit.

## [Unreleased]

- Browse many Git repositories in one native window with Onevra's default colors, a collapsible, resizable sidebar and its own app icon; it reopens where it was, with the same size and sidebar layout.
- Choose the project parent directory, theme and language in Settings, saved under `~/.onegit/`; an earlier `~/.oneo/onegit/` directory moves there and legacy settings are imported.
- Scan the parent directory in the background into a project tree that marks missing paths and remembers expansion.
- Pin projects to a flat, name-sorted group; both sidebar groups collapse independently.
- Find projects and open Settings from the command palette (`⌘E`).
- Switch between a project's worktrees with tabs, returning to the last project and worktree.
- Fetch, pull, push, stash and switch branches from the title bar, with fetch, pull and push dialogs and commit counts; fetch updates every remote or one remote branch.
- Browse current changes, all commits and filterable branches, remote branches and tags in a project sidebar, jumping to their commits or deleting them from their menu.
- See all commits in a colored graph across all refs, with collapsible merges, find, author avatars, and floating commit details with diffs, image previews and file contents.
- Avoid the UI freezing while choosing the project parent directory.
