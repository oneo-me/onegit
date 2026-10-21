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
- Paths that no longer exist are dropped with their pinning and removal, as are folders they leave empty. A missing parent directory, such as an unmounted volume, keeps the cache.
- A stale scan for a previous directory never overwrites the current one.

State is saved after a merge, after pin, group or expansion changes and when the window closes normally.

## Sidebar groups

The sidebar shows collapsible groups, all expanded by default: Pinned first, then the user's groups and Ungrouped Projects in the user's order.

- Pinned and user groups list their projects flat, sorted by name case-insensitively, then by full path.
- Ungrouped Projects holds every other project in the directory hierarchy, hiding folders that only lead to pinned or grouped projects.
- Pinning wins over a group; unpinning returns a project to its group. A project naming a deleted group is ungrouped.
- Groups are a view over the cached nodes. They do not change the tree, selection or expansion.

Group headers offer New Group; user groups also rename or delete, which returns their projects to Ungrouped Projects. Dragging a group header onto another group moves it there; Pinned stays first.

Pinning, group membership and icons live in the cached nodes, and the group order, names and collapse in `groups`, all per parent directory. Pinned and Ungrouped Projects share their collapse across parent directories.

Nodes start collapsed. The sidebar follows Neorio's Git window: Onevra `SidebarItem` rows in a `ScrollView`, and `Collapsible` headings that count their projects.

- Repositories lead with their chosen icon, else a blank placeholder tile; folders with a folder icon.
- Clicking a folder, Right and Left open and close it; folders are never selected.

## Context menus

Right-clicking a folder offers Open Folder. A repository row also offers pinning, Move to Group, which can make a new group for it, Choose Icon, Reset Icon once one is set, and Remove Project.

- The icon picker opens in the project's folder and accepts PNG, JPEG, GIF, WebP, BMP and SVG. An image that does not decode leaves the icon unchanged and shows a toast.
- The node stores the image's path. Images load in the background on first use; an unreadable one shows the placeholder.

- Removal hides the project from every group, the command palette and selection fallback without touching its files.
- The flag lives in the cached tree, so scans and restarts keep the project hidden.
- The toast after a removal undoes it; the palette's Restore Removed Projects command brings back every project removed from the current parent directory.
- Opening a folder runs `Shell.OpenPath`; a failure shows as a toast.

## Command palette

The palette searches every cached project for the current parent directory, including collapsed ones but not removed ones. It matches names and paths and opens the same node the sidebar would. While a project is hidden, it also lists Restore Removed Projects.

The search box above the sidebar and `⌘F` open it. It also lists theme, language and parent directory commands that open the matching settings row. Onegit owns the command data; Onevra `CommandDialog` owns filtering, keyboard navigation and focus. The palette does not open over the settings dialog or the native directory picker.
