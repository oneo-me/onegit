package gitview

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
)

// opRestore is the Git write resetting a file to a commit's version.
const opRestore = "restore"

// fileActions adds opening and showing the chosen file to g: one file at a
// time, disabled while a directory or several files are chosen.
func (v *View) fileActions(g *menuGroups, p *filePane, files []FileChange) {
	one := len(files) == 1
	if v.opts.OpenFile != nil {
		if g.item(v.text("open_file")).Disabled(!one).Chosen() && one {
			v.openFile(p, files[0])
		}
	}
	if v.opts.RevealFile != nil {
		if g.item(v.text("show_in_file_manager")).Disabled(!one).Chosen() && one {
			v.revealFile(p, files[0])
		}
	}
}

// copyFileActions adds copying the chosen files' paths to g, relative to the
// worktree's top or absolute.
func (v *View) copyFileActions(c *ui.Context, g *menuGroups, p *filePane, files []FileChange) {
	if g.item(v.text("copy_paths")).Chosen() {
		var paths []string
		for _, f := range files {
			paths = append(paths, f.Path)
		}
		c.WriteClipboard(strings.Join(paths, "\n"))
	}
	if g.item(v.text("copy_absolute_paths")).Chosen() {
		var paths []string
		for _, f := range files {
			paths = append(paths, filepath.Join(p.changes.Top, filepath.FromSlash(f.Path)))
		}
		c.WriteClipboard(strings.Join(paths, "\n"))
	}
}

// allFilesMenu fills the menu of a file in a commit's or stash's files: the
// common file actions, saving one file, and copying the chosen paths. A
// commit's Changes also resets a file to the commit's version or switches to
// its Files tab.
func (v *View) allFilesMenu(c *ui.Context, row ui.Element, p *filePane, area Area, key string) {
	row.ContextMenu(func(m *ui.Menu) {
		files := p.chosenFiles(area, key)
		one := len(files) == 1
		g := &menuGroups{m: m}
		v.fileActions(g, p, files)
		if p == v.history.changes {
			if _, entry := entryID(v.history.selected); !entry {
				g.group()
				v.restoreItems(g, v.history.selected, files)
			}
		}
		g.group()
		if one {
			if g.item(v.text("save_file_as")).Chosen() {
				v.saveFileChange(p, files[0])
			}
			if p == v.history.changes && files[0].Status != 'D' {
				if g.item(v.text("show_in_files")).Chosen() {
					v.showInFiles(files[0].Path)
				}
			}
		}
		if len(files) > 0 {
			g.group()
			v.copyFileActions(c, g, p, files)
		}
	})
}

// restoreItems adds a submenu resetting the one chosen file to its state at
// the commit or before it.
func (v *View) restoreItems(g *menuGroups, commit string, files []FileChange) {
	if len(files) != 1 {
		return
	}
	i := commitIndex(v.repo.commits, commit)
	if i < 0 {
		return
	}
	c := v.repo.commits[i]
	file, can := files[0], v.CanSync()
	sub := g.submenu(v.text("restore_file"), func(m *ui.Menu) {
		if m.Item(v.text("restore_file_state")).Disabled(!can).Chosen() && can {
			v.openRestoreFile(c.Hash, c.Short, file.Path, false)
		}
		if len(c.Parents) > 0 && m.Item(v.text("restore_file_before")).Disabled(!can).Chosen() && can {
			v.openRestoreFile(c.Hash, c.Short, file.Path, true)
		}
	})
	sub.Disabled(!can)
}

// restoring confirms resetting a file to the version a commit holds.
type restoring struct {
	open          bool
	commit, short string
	path          string
	// before resets to the commit's first parent instead of the commit.
	before bool
}

// openRestoreFile asks whether to reset path to commit's version, or its
// first parent's.
func (v *View) openRestoreFile(commit, short, path string, before bool) {
	if v.CanSync() {
		v.restoring = restoring{open: true, commit: commit, short: short, path: path, before: before}
		v.invalidate()
	}
}

// restoreFileDialog asks before resetting a file, whose changes cannot be
// undone.
func (v *View) restoreFileDialog(c *ui.Context) {
	d := &v.restoring
	if !d.open {
		return
	}
	args := map[string]any{"Path": d.path, "Commit": d.short}
	title := v.textf("restore_file_title", args)
	if d.before {
		title = v.textf("restore_file_before_title", args)
	}
	components.Dialog(c, &d.open, components.DialogOptions{Label: title, Inset: v.dialogInset(c)}, func(panel ui.Element) {
		t := c.Theme()
		panel.Role(ui.RoleAlertDialog)
		components.Text(c, title).Bold().Wrap().FillWidth()
		components.Text(c, v.text("restore_file_message")).Wrap().FillWidth().TextColor(t.TextMuted)
		ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
			if components.Button(c, v.text("cancel"), components.ButtonSecondary).AutoFocus().Clicked() {
				d.open = false
			}
			if components.Button(c, v.text("restore_file_confirm"), components.ButtonPrimary).Disabled(!v.CanSync()).Clicked() {
				d.open = false
				commit, file, before := d.commit, d.path, d.before
				v.syncRepository(opRestore, v.text("restore_file_failed"), func(git Git, path string) error {
					top, err := git.Top(path)
					if err != nil {
						return err
					}
					if before {
						commit += "^"
					}
					return git.RestoreFile(top, commit, file)
				})
			}
		})
	})
}

// showInFiles shows a commit's Files tab with path selected.
func (v *View) showInFiles(file string) {
	h := &v.history
	if h.selected == "" {
		return
	}
	h.detail = detailFiles
	v.selectCommit(h.selected)
	// A read started above arranges the files after it finishes, keeping
	// this selection; an already read tree needs the row revealed here.
	h.files.selected = fileKey(FileChange{Path: file})
	if !h.files.loading && h.files.loaded && h.files.key == h.selected {
		v.revealPaneFile(h.files, file)
	}
}

// revealPaneFile selects path in p, opening the directories that hide it and
// scrolling it into view.
func (v *View) revealPaneFile(p *filePane, file string) {
	key := fileKey(FileChange{Path: file})
	if n := p.tree.nodes[key]; n == nil || n.children != nil {
		return
	}
	area := keyArea(key)
	l := &p.lists[area]
	for _, dir := range p.tree.ancestors(key) {
		l.state.Open.Add(dir)
	}
	p.selected = key
	l.cursor = p.tree.row(p.roots(area), key, l.state.Open.Has)
	if l.cursor >= 0 {
		l.state.List.ScrollIntoView(l.cursor)
	}
	if fileKey(p.file) != key {
		v.loadFile(p)
	}
}

// openFile opens f's version with the default application: the worktree's
// own file for its changes, or a temporary copy of the version a commit or
// stash holds.
func (v *View) openFile(p *filePane, f FileChange) {
	if v.opts.OpenFile == nil {
		return
	}
	title, changes, current := v.text("open_file_failed"), p.changes, p == v.current
	go func() {
		if current {
			full := filepath.Join(changes.Top, filepath.FromSlash(f.Path))
			if _, err := v.git.lstat(full); err == nil {
				if err = v.opts.OpenFile(full); err != nil {
					v.showFileActionError(title, err)
				}
				return
			}
		}
		before, after, err := v.git.FileVersions(changes, f)
		if err == nil {
			data := after
			if f.Status == 'D' || data == nil {
				data = before
			}
			var file string
			if file, err = v.writeTempFile(f.Path, data); err == nil {
				err = v.opts.OpenFile(file)
			}
		}
		if err != nil {
			v.showFileActionError(title, err)
		}
	}()
}

// writeTempFile writes data as name in a temporary directory, which Close
// removes.
func (v *View) writeTempFile(name string, data []byte) (string, error) {
	dir, err := os.MkdirTemp("", "onegit-*")
	if err != nil {
		return "", err
	}
	v.tempMu.Lock()
	v.tempDirs = append(v.tempDirs, dir)
	v.tempMu.Unlock()
	path := filepath.Join(dir, filepath.Base(filepath.FromSlash(name)))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// removeTempDirs deletes the temporary directories of files opened from
// commits or stashes.
func (v *View) removeTempDirs() {
	v.tempMu.Lock()
	dirs := v.tempDirs
	v.tempDirs = nil
	v.tempMu.Unlock()
	for _, dir := range dirs {
		os.RemoveAll(dir)
	}
}

// revealFile shows f's worktree path in the file manager, falling back to
// its directory or the worktree's top when the version shown is not there.
func (v *View) revealFile(p *filePane, f FileChange) {
	if v.opts.RevealFile == nil {
		return
	}
	top := p.changes.Top
	go func() {
		full := filepath.Join(top, filepath.FromSlash(f.Path))
		if _, err := v.git.lstat(full); err != nil {
			if dir := filepath.Dir(full); dir != full {
				if _, err := v.git.lstat(dir); err == nil {
					full = dir
				} else {
					full = top
				}
			} else {
				full = top
			}
		}
		v.opts.RevealFile(full)
	}()
}

// saveFileChange asks where to save f's version and writes it there.
func (v *View) saveFileChange(p *filePane, f FileChange) {
	title, name, changes := v.text("save_file_title"), filepath.Base(f.Path), p.changes
	failed := v.text("save_file_failed")
	go func() {
		before, after, err := v.git.FileVersions(changes, f)
		if err == nil {
			data := after
			if f.Status == 'D' || data == nil {
				data = before
			}
			_, err = v.saveFile(title, name, data)
		}
		if err != nil {
			v.showFileActionError(failed, err)
		}
	}()
}

// showFileActionError reports a file action's failure as a toast.
func (v *View) showFileActionError(title string, err error) {
	v.dispatch(func() {
		if v.closed {
			return
		}
		v.toast = &ui.Toast{Title: title, Description: err.Error(), Type: components.ToastError}
		v.invalidate()
	})
}
