# Project discovery

Project discovery turns one user-chosen parent directory into the sidebar tree. It scans once per startup or directory change, caches the result per parent directory and never watches the file system.

## Scanning

The scan walks the parent directory recursively in the background. Each candidate is confirmed with `git`, which resolves the real repository root.

- Recognized: regular repositories, linked worktrees, initialized submodules and bare repositories.
- Skipped: `.git` internals, symlinked directories, plain folders and invalid `.git` entries.
- Recursion stops at the first valid repository, so nested repositories and submodules are not listed.
- Each Git check has a timeout. Changing the directory or closing the window cancels the scan.
- Only repositories and the folders leading to them are kept. Paths, not display names, identify nodes.

The scan runs silently with no progress text. A failed directory keeps the projects already found and shows the first error.

## Cache and merging

The cache is keyed by parent directory, so switching directories restores each tree independently. Scan results merge on the main thread by path.

- New nodes are added; existing nodes and their expansion are kept.
- Paths that no longer exist are marked missing and cleared when they reappear.
- A stale scan for a previous directory never overwrites the current one.

State is saved after a merge, after pin or expansion changes and when the window closes normally.

## Sidebar groups

The sidebar shows two collapsible groups, Pinned and Other Projects, both expanded by default.

- Pinned lists pinned projects flat, sorted by name case-insensitively, then by full path.
- Other Projects keeps the directory hierarchy and hides folders that only lead to pinned projects.
- Groups are a view over the cached nodes. They do not change the tree, selection or expansion.
- Pinning is stored per parent directory. Group collapse is shared across parent directories.

Nodes start collapsed. The tree uses Onevra `Tree` inside a `Sidebar` in a `ScrollView`, and the group headers use `Collapsible`.

## Context menus

Right-clicking a folder offers Open Folder. A repository row also offers pinning and Remove Project.

- Removal hides the project from both groups, the command palette and selection fallback without touching its files.
- The flag lives in the cached tree, so scans and restarts keep the project hidden.
- The toast after a removal undoes it; the palette's Restore Removed Projects command brings back every project removed from the current parent directory.
- Opening a folder runs `Shell.OpenPath`; a failure shows as a toast.

## Command palette

The palette searches every cached project for the current parent directory, including collapsed and missing ones but not removed ones. It matches names and paths and opens the same node the sidebar would. While a project is hidden, it also lists Restore Removed Projects.

It also lists theme, language and parent directory commands that open the matching settings row. Onegit owns the command data; Onevra `CommandDialog` owns filtering, keyboard navigation and focus. The palette does not open over the settings dialog or the native directory picker.
