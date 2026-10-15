package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"
)

// Sections of the project sidebar. The value is persisted, so treat it as a
// stored key independent of the translated label.
const (
	sectionChanges  = "changes"
	sectionCommits  = "commits"
	sectionBranches = "branches"
	sectionSearch   = "search"
)

// Tabs of the commit details pane.
const (
	detailCommit = iota
	detailChanges
	detailTree
)

type gitJob struct {
	running bool
	label   string
	problem string
	version uint64
	cancel  context.CancelFunc
}

// repositoryState owns the project sidebar selection and the Git reads that
// back the toolbar and the content views.
type repositoryState struct {
	section      string
	sidebarWidth float32
	detailSplit  float32
	dispatch     func(func())

	status   loadState[statusSnapshot]
	branches loadState[[]branchEntry]
	commits  loadState[[]commitEntry]
	files    loadState[[]commitFile]
	refs     loadState[sidebarRefs]

	detailTab  int
	filter     string
	treeGroups map[string]*bool

	changesList components.ListState
	branchList  components.ListState
	commitList  components.ListState
	commitIndex int
	query       string
	visible     []commitEntry

	job gitJob
}

func (r *repositoryState) reset() {
	r.status.reset()
	r.branches.reset()
	r.commits.reset()
	r.files.reset()
	r.refs.reset()
	r.cancelJob()
	r.commitIndex = -1
	r.visible = nil
	r.changesList = components.ListState{}
	r.branchList = components.ListState{}
	r.commitList = components.ListState{}
}

func (r *repositoryState) cancelJob() {
	if r.job.cancel != nil {
		r.job.cancel()
	}
	r.job = gitJob{version: r.job.version + 1}
}

func (s *settings) repositoryDispatch() func(func()) {
	if s.repo.dispatch != nil {
		return s.repo.dispatch
	}
	return mygo.RunOnMain
}

func (s *settings) invalidate() {
	if s.window != nil {
		s.window.Invalidate()
	}
}

func (s *settings) sectionFor(node *projectNode) string {
	if node != nil && node.Repository {
		if saved := s.state.SelectedSections[node.Path]; saved != "" {
			return saved
		}
	}
	return sectionChanges
}

// repositoryChanged reloads every dataset after the active worktree or project
// changed. The sidebar section is navigation state and survives the change.
func (s *settings) repositoryChanged() {
	s.repo.reset()
	s.ensureRepositoryData()
}

// refreshRepository re-reads everything for the current worktree, for example
// from the toolbar's refresh action. It also clears any previous job result.
func (s *settings) refreshRepository() {
	s.repo.reset()
	s.ensureRepositoryData()
}

// reloadRepositoryData re-reads the active worktree while keeping the job
// result, so a failed fetch or push stays visible until the next action.
func (s *settings) reloadRepositoryData() {
	s.repo.status.reset()
	s.repo.branches.reset()
	s.repo.commits.reset()
	s.ensureRepositoryData()
}

// ensureRepositoryData starts the reads the current section needs. Reads for a
// path are started once and reused while the worktree stays selected.
func (s *settings) ensureRepositoryData() {
	dir := s.overview.SelectedPath
	if dir == "" {
		return
	}
	dispatch := s.repositoryDispatch()
	if s.repo.status.needs(dir) {
		s.repo.status.start(dispatch, dir, func(ctx context.Context) (statusSnapshot, error) {
			return readStatus(ctx, dir)
		}, s.invalidate)
	}
	if s.repo.refs.needs(dir) {
		s.repo.refs.start(dispatch, dir, func(ctx context.Context) (sidebarRefs, error) {
			return readSidebarRefs(ctx, dir)
		}, s.invalidate)
	}
	if s.repo.branches.needs(dir) {
		s.repo.branches.start(dispatch, dir, func(ctx context.Context) ([]branchEntry, error) {
			return readBranches(ctx, dir)
		}, s.invalidate)
	}
	switch s.repo.section {
	case sectionCommits, sectionSearch, sectionBranches:
		if s.repo.commits.needs(dir) {
			s.repo.commits.start(dispatch, dir, func(ctx context.Context) ([]commitEntry, error) {
				return readCommits(ctx, dir, 300)
			}, s.invalidate)
		}
	}
}

func (s *settings) setSection(section string) {
	if s.repo.section == section {
		return
	}
	s.repo.section = section
	if s.selectedProject != nil && s.selectedProject.Repository {
		if s.state.SelectedSections == nil {
			s.state.SelectedSections = make(map[string]string)
		}
		s.state.SelectedSections[s.selectedProject.Path] = section
		s.saveSidebarState()
	}
	s.ensureRepositoryData()
}

// runGitJob runs a network Git command and refreshes reads when it finishes.
func (s *settings) runGitJob(label string, args ...string) {
	dir := s.overview.SelectedPath
	if dir == "" || s.repo.job.running {
		return
	}
	s.repo.cancelJob()
	version := s.repo.job.version
	s.repo.job.running = true
	s.repo.job.label = label
	s.repo.job.problem = ""
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	s.repo.job.cancel = cancel
	dispatch := s.repositoryDispatch()
	go func() {
		defer cancel()
		_, err := runGitAction(ctx, dir, args...)
		dispatch(func() {
			if s.repo.job.version != version {
				return
			}
			s.repo.job.running = false
			s.repo.job.cancel = nil
			if err != nil {
				s.repo.job.problem = err.Error()
			}
			s.reloadRepositoryData()
			s.invalidate()
		})
	}()
}

// welcome keeps the previous entry point; it now builds the repository page.
func (s *settings) welcome(c *ui.Context) {
	if s.selectedProject == nil || !s.selectedProject.Repository {
		return
	}
	s.repositoryPage(c)
}

func (s *settings) repositoryPage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(sidebar.HeaderHeight(c), 0, 0, 0).Children(func() {
		s.worktreeTabs(c)
		s.repositoryToolbar(c)
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		if s.overview.Loading {
			s.repositoryMessage(c, s.text("正在读取 worktree…"))
			return
		}
		if s.overview.Problem != "" {
			s.repositoryMessage(c, s.text("读取 worktree 失败：")+s.overview.Problem)
			return
		}
		if s.repo.sidebarWidth < 1 {
			s.repo.sidebarWidth = 220
		}
		if s.repo.detailSplit < 1 {
			s.repo.detailSplit = 260
		}
		split := components.Split(c, &s.repo.sidebarWidth, func() { s.projectSidebar(c) }, func() { s.repositoryContent(c) }, components.SplitOptions{
			FirstMin: 180, SecondMin: 280,
		})
		split.Grow(1).MinHeight(0)
	})
}

func (s *settings) repositoryMessage(c *ui.Context, message string) {
	ui.Column(c).Grow(1).FillWidth().AlignItems(ui.Center).Justify(ui.Center).Padding(16).Children(func() {
		components.Text(c, message).Wrap().TextColor(c.Theme().TextMuted)
	})
}

// repositoryToolbar is the strip below the worktree tabs and above the project
// sidebar: common Git actions on the left, the active branch in the middle and
// navigation actions on the right, as in the reference application.
func (s *settings) repositoryToolbar(c *ui.Context) {
	t := c.Theme()
	running := s.repo.job.running
	ui.Row(c).FillWidth().Shrink(0).Height(t.Space(12)).AlignItems(ui.Center).
		Background(t.Background).Padding(0, t.Space(1)).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(.5)).Children(func() {
			s.toolbarButton(c, lucide.Search(), s.text("快速启动"), false, s.openCommands)
			s.toolbarButton(c, lucide.Download(), s.text("获取"), running, func() {
				s.runGitJob(s.text("正在获取…"), "fetch", "--all", "--prune")
			})
			s.toolbarButton(c, lucide.ArrowDownToLine(), s.text("拉取"), running, func() {
				s.runGitJob(s.text("正在拉取…"), "pull")
			})
			s.toolbarButton(c, lucide.ArrowUpFromLine(), s.text("推送"), running, func() {
				s.runGitJob(s.text("正在推送…"), "push")
			})
			s.toolbarButton(c, lucide.Archive(), s.text("贮藏"), running, func() {
				s.runGitJob(s.text("正在贮藏…"), "stash", "push", "--include-untracked")
			})
		})
		s.statusPill(c)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Justify(ui.End).Gap(t.Space(.5)).Children(func() {
			s.toolbarMenuButton(c, lucide.GitBranch(), s.text("分支操作"), running, s.branchMenu)
		})
	})
}

// statusPill is the rounded branch summary centred in the toolbar.
func (s *settings) statusPill(c *ui.Context) {
	t := c.Theme()
	name := ""
	if s.selectedProject != nil {
		name = filepath.Base(s.selectedProject.Path)
	}
	branch := s.currentBranch()
	if s.repo.job.running {
		branch = s.repo.job.label
	} else if s.repo.job.problem != "" {
		branch = s.text("操作失败")
	}
	head := s.repo.status.value.Head
	pill := ui.Column(c).Width(t.Space(75)).Height(t.Space(9)).Shrink(0).Radius(t.Space(2)).
		Background(t.Surface).Justify(ui.Center).AlignItems(ui.Center).Gap(t.Space(.25)).Children(func() {
		components.Text(c, name).FontSize(t.Rem(.8)).Bold().SingleLine()
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
			components.Icon(c, lucide.GitBranch()).Size(t.Space(3), t.Space(3)).TextColor(t.TextMuted)
			components.Text(c, branch).FontSize(t.Rem(.75)).TextColor(t.TextMuted).SingleLine()
			if head.Upstream != "" && (head.Ahead > 0 || head.Behind > 0) {
				components.Text(c, fmt.Sprintf("↑%d ↓%d", head.Ahead, head.Behind)).FontSize(t.Rem(.7)).TextColor(t.TextMuted).SingleLine()
			}
		})
	})
	if s.repo.job.problem != "" {
		components.Tooltip(c, pill, s.repo.job.problem)
	}
}

func (s *settings) branchMenu(m *ui.Menu) {
	if !s.repo.branches.loaded {
		m.Item(s.text("正在读取…")).Disabled(true)
		return
	}
	count := 0
	for _, branch := range s.repo.branches.value {
		if branch.Remote {
			continue
		}
		count++
		if m.Item(branch.Name).Checked(branch.Current).Chosen() && !branch.Current {
			s.runGitJob(s.text("正在切换…"), "switch", branch.Name)
		}
	}
	if count == 0 {
		m.Item(s.text("没有分支。")).Disabled(true)
	}
}

func (s *settings) currentBranch() string {
	if s.repo.status.loaded {
		head := s.repo.status.value.Head
		switch {
		case head.Detached:
			return s.text("游离 HEAD")
		case head.Branch != "":
			return head.Branch
		}
	}
	if i := s.overview.selectedIndex; i >= 0 && i < len(s.overview.Worktrees) {
		item := s.overview.Worktrees[i]
		if item.Detached {
			return s.text("游离 HEAD")
		}
		if item.Branch != "" {
			return item.Branch
		}
	}
	return ""
}

func (s *settings) branchStatusLabel() string {
	if s.repo.job.running {
		return s.repo.job.label
	}
	if s.repo.job.problem != "" {
		return s.text("Git 操作失败：") + s.repo.job.problem
	}
	if s.repo.status.loaded {
		head := s.repo.status.value.Head
		switch {
		case head.Detached:
			return s.text("游离 HEAD")
		case head.Branch != "":
			label := head.Branch
			if head.Unborn {
				label += s.text("（尚无提交）")
			}
			if head.Upstream != "" {
				label += " → " + head.Upstream
				if head.Ahead > 0 || head.Behind > 0 {
					label += fmt.Sprintf(" ↑%d ↓%d", head.Ahead, head.Behind)
				}
			}
			return label
		}
	}
	return s.currentBranch()
}

func (s *settings) projectSidebar(c *ui.Context) {
	t := c.Theme()
	name := ""
	if s.selectedProject != nil {
		name = filepath.Base(s.selectedProject.Path)
	}
	ui.Column(c).FillHeight().FillWidth().Background(t.Surface).Clip().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(t.Space(1)).
			Padding(t.Space(2), t.Space(1.5), t.Space(1), t.Space(2)).Children(func() {
			components.Text(c, name).Bold().FontSize(t.Rem(.95)).Grow(1).MinWidth(0).SingleLine()
			sidebar.IconButton(c, lucide.Settings(), s.text("仓库设置")).Menu(func(m *ui.Menu) {
				if m.Item(s.text("刷新")).Chosen() {
					s.refreshRepository()
				}
				if m.Item(s.text("在文件管理器中显示")).Chosen() && s.overview.SelectedPath != "" {
					mygo.Shell.ShowItemInFolder(s.overview.SelectedPath)
				}
			})
		})
		s.sidebarPrimary(c, sectionChanges, s.text("更改"), lucide.FileDiff(), s.changesCountLabel())
		s.sidebarPrimary(c, sectionCommits, s.text("全部提交"), lucide.GitCommitHorizontal(), "")
		ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Gap(t.Space(1)).
			Padding(t.Space(1), 0).Border(1, t.Border).BorderWidth(0, 0, 1, 0).Children(func() {
			s.sidebarToggle(c, sectionBranches, lucide.GitBranch(), s.text("分支"))
			s.sidebarToggle(c, sectionSearch, lucide.Search(), s.text("搜索"))
		})
		components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
			ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(1), 0, t.Space(2), 0).Children(func() {
				s.branchTree(c)
			})
		})
		ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(1), t.Space(1.5), t.Space(2), t.Space(1.5)).Children(func() {
			components.TextBox(c, &s.repo.filter, components.TextBoxOptions{LeadingIcon: lucide.Search()}).
				FillWidth().Placeholder(s.text("筛选…")).Label(s.text("筛选"))
		})
	})
}

func (s *settings) sidebarPrimary(c *ui.Context, id, label string, icon *ui.SVG, suffix string) {
	t := c.Theme()
	item := components.SidebarItemWithLeading(c, label+suffix, s.repo.section == id, func(c *ui.Context, size float32) {
		if icon != nil {
			components.Icon(c, icon).Size(size, size).TextColor(t.TextMuted)
		}
	}).Key(id)
	item.Padding(t.Space(1.5), t.Space(2))
	if item.Clicked() {
		s.setSection(id)
	}
}

func (s *settings) sidebarToggle(c *ui.Context, id string, icon *ui.SVG, label string) {
	t := c.Theme()
	on := s.repo.section == id
	b := ui.ButtonBase(c).Key("toggle-"+id).Size(t.Space(9), t.Space(7)).Radius(t.Radius).
		Label(label).TextColor(t.TextMuted).AlignItems(ui.Center).Justify(ui.Center)
	if on {
		b.Background(t.Text.Alpha(.1)).TextColor(t.Text)
	}
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if !on && b.Hovered() {
			p.Fill(r, t.Text.Alpha(.06), t.Radius)
		}
	}).Children(func() {
		components.Icon(c, icon).Size(t.Space(4), t.Space(4))
	})
	components.Tooltip(c, b, label)
	if b.Clicked() {
		s.setSection(id)
	}
}

// groupOpen keeps a sidebar group's expansion across frames and worktrees.
func (s *settings) groupOpen(key string, defaultOpen bool) *bool {
	if s.repo.treeGroups == nil {
		s.repo.treeGroups = map[string]*bool{}
	}
	if open, ok := s.repo.treeGroups[key]; ok {
		return open
	}
	open := defaultOpen
	s.repo.treeGroups[key] = &open
	return &open
}

func (s *settings) branchTree(c *ui.Context) {
	match := func(name string) bool {
		if s.repo.filter == "" {
			return true
		}
		return strings.Contains(strings.ToLower(name), strings.ToLower(s.repo.filter))
	}
	var local, remote []string
	if s.repo.branches.loaded {
		for _, branch := range s.repo.branches.value {
			if !match(branch.Name) {
				continue
			}
			if branch.Remote {
				remote = append(remote, branch.Name)
			} else {
				local = append(local, branch.Name)
			}
		}
	}
	var refs sidebarRefs
	if s.repo.refs.loaded {
		refs = s.repo.refs.value
	}
	filtered := func(items []string) []string {
		result := make([]string, 0, len(items))
		for _, item := range items {
			if match(item) {
				result = append(result, item)
			}
		}
		return result
	}
	components.Tree(c, func() {
		s.treeSection(c, "branches", s.text("本地分支"), local, true)
		s.treeSection(c, "remotes", s.text("远程分支"), remote, true)
		s.treeSection(c, "tags", s.text("标签"), filtered(refs.Tags), false)
		s.treeSection(c, "stashes", s.text("贮藏"), filtered(refs.Stashes), false)
		s.treeSection(c, "submodules", s.text("子模块"), filtered(refs.Submodules), false)
	})
}

func (s *settings) treeSection(c *ui.Context, key, label string, items []string, defaultOpen bool) {
	ui.Column(c).Key("group-" + key).FillWidth().Shrink(0).Children(func() {
		components.TreeItem(c, label, s.groupOpen(key, defaultOpen), false, func() {
			for _, name := range items {
				ui.Column(c).Key("leaf-" + key + "-" + name).FillWidth().Shrink(0).Children(func() {
					components.TreeItem(c, name, nil, false, nil)
				})
			}
		})
	})
}

func (s *settings) changesCountLabel() string {
	if s.repo.status.loaded && len(s.repo.status.value.Changes) > 0 {
		return fmt.Sprintf(" (%d)", len(s.repo.status.value.Changes))
	}
	return ""
}

func (s *settings) repositoryContent(c *ui.Context) {
	switch s.repo.section {
	case sectionCommits:
		s.commitsView(c, false)
	case sectionBranches:
		s.branchesView(c)
	case sectionSearch:
		s.searchView(c)
	default:
		s.changesView(c)
	}
}

func (s *settings) contentState[T any](c *ui.Context, state *loadState[T]) bool {
	if state.problem != "" {
		components.Text(c, s.text("读取失败：")+state.problem).Wrap().TextColor(c.Theme().TextMuted).Padding(16)
		return false
	}
	if state.loading && !state.loaded {
		components.Text(c, s.text("正在读取…")).TextColor(c.Theme().TextMuted).Padding(16)
		return false
	}
	return true
}

func (s *settings) changesView(c *ui.Context) {
	ui.Column(c).Fill().Padding(0).Children(func() {
		if !s.contentState(c, &s.repo.status) {
			return
		}
		changes := s.repo.status.value.Changes
		if len(changes) == 0 {
			s.repositoryMessage(c, s.text("没有未提交的更改。"))
			return
		}
		components.List(c, &s.repo.changesList, len(changes), func(i int) {
			s.changeRow(c, changes[i])
		}).Fill()
	})
}

func (s *settings) changeRow(c *ui.Context, entry changeEntry) {
	t := c.Theme()
	index, work := entry.Index, entry.Work
	if index == " " {
		index = "·"
	}
	if work == " " {
		work = "·"
	}
	ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Padding(t.Space(1), t.Space(2.5)).Children(func() {
		components.Text(c, index).FontSize(t.Rem(.85)).TextColor(changeColor(t, entry.Index)).Shrink(0)
		components.Text(c, work).FontSize(t.Rem(.85)).TextColor(changeColor(t, entry.Work)).Shrink(0)
		path := entry.Path
		if entry.Orig != "" {
			path = entry.Orig + " → " + entry.Path
		}
		components.Text(c, path).SingleLine().Grow(1).MinWidth(0)
	})
}

func changeColor(t *ui.Theme, code string) ui.Color {
	switch code {
	case "A", "?":
		return t.Success
	case "D":
		return t.Danger
	case "R", "C":
		return t.Accent
	case "M", "T", "U":
		return t.Warning
	default:
		return t.TextMuted
	}
}

func (s *settings) commitsView(c *ui.Context, filtered bool) {
	if !s.contentState(c, &s.repo.commits) {
		return
	}
	s.repo.visible = s.filteredCommits(filtered)
	if len(s.repo.visible) == 0 {
		s.repositoryMessage(c, s.text("没有提交。"))
		return
	}
	if s.repo.commitIndex >= len(s.repo.visible) {
		s.repo.commitIndex = len(s.repo.visible) - 1
	}
	if s.repo.commitIndex < 0 {
		s.repo.commitIndex = 0
	}
	split := components.Split(c, &s.repo.detailSplit, func() {
		s.commitList(c)
	}, func() {
		s.commitDetails(c, s.repo.visible[s.repo.commitIndex])
	}, components.SplitOptions{Orientation: components.Vertical, FirstMin: 110, SecondMin: 150})
	split.FillWidth().Grow(1).MinHeight(0)
}

func (s *settings) commitList(c *ui.Context) {
	rows, columns := buildGraph(s.repo.visible)
	components.List(c, s.commitListState(), len(s.repo.visible), func(i int) {
		s.commitRow(c, s.repo.visible[i], rows[i], columns)
	}).Fill()
}

func (s *settings) commitListState() *components.ListState {
	list := &s.repo.commitList
	list.Selected = &s.repo.commitIndex
	list.Key = func(i int) any { return s.repo.visible[i].Hash }
	list.Label = func(i int) string { return s.repo.visible[i].Subject }
	list.OnChange = func() { s.ensureCommitFiles() }
	return list
}

// commitRow is one revision: graph, decorations, subject, author, hash and date.
func (s *settings) commitRow(c *ui.Context, entry commitEntry, row graphRow, columns int) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinWidth(0).Height(t.Space(5.75)).AlignItems(ui.Center).Children(func() {
		graphCell(c, row, columns, len(entry.Parents) > 1)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1)).
			Padding(0, t.Space(1.5)).Children(func() {
			refPills(c, entry.Refs)
			components.Text(c, entry.Subject).SingleLine().Grow(1).MinWidth(0).FontSize(t.Rem(.82))
			authorAvatar(c, entry.Author, t.Space(4))
			components.Text(c, entry.Author).Width(t.Space(28)).Shrink(0).SingleLine().
				FontSize(t.Rem(.78)).TextColor(t.TextMuted)
			components.Text(c, entry.Short).Width(t.Space(18)).Shrink(0).
				FontSize(t.Rem(.78)).TextColor(t.TextMuted)
			components.Text(c, s.dateLabel(entry.Date)).Width(t.Space(33)).Shrink(0).
				FontSize(t.Rem(.78)).TextColor(t.TextMuted).TextAlign(ui.End)
		})
	})
}

// ensureCommitFiles loads the changed paths of the selected commit for the
// Changes and File Tree details tabs.
func (s *settings) ensureCommitFiles() {
	if s.repo.detailTab == detailCommit {
		return
	}
	dir := s.overview.SelectedPath
	if dir == "" || s.repo.commitIndex < 0 || s.repo.commitIndex >= len(s.repo.visible) {
		return
	}
	entry := s.repo.visible[s.repo.commitIndex]
	if s.repo.files.needs(entry.Hash) {
		s.repo.files.start(s.repositoryDispatch(), entry.Hash, func(ctx context.Context) ([]commitFile, error) {
			return readCommitFiles(ctx, dir, entry.Hash)
		}, s.invalidate)
	}
}

func (s *settings) setDetailTab(tab int) {
	s.repo.detailTab = tab
	s.ensureCommitFiles()
}

func (s *settings) commitDetails(c *ui.Context, entry commitEntry) {
	t := c.Theme()
	s.ensureCommitFiles()
	ui.Column(c).FillWidth().FillHeight().Background(t.Background).Children(func() {
		tabs := components.Tabs(c, &s.repo.detailTab, s.text("提交"), s.text("更改"), s.text("文件树"))
		tabs.List.FillWidth().Shrink(0).Padding(t.Space(1), t.Space(1))
		ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border)
		if tabs.List.Changed() {
			s.setDetailTab(s.repo.detailTab)
		}
		switch s.repo.detailTab {
		case detailChanges:
			s.commitFileList(c)
		case detailTree:
			s.commitFileTree(c)
		default:
			s.commitSummary(c, entry)
		}
	})
}

func (s *settings) detailField(c *ui.Context, label string, body func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Start).Gap(t.Space(2)).Children(func() {
		components.Text(c, label).Width(t.Space(20)).Shrink(0).FontSize(t.Rem(.7)).
			TextColor(t.TextMuted).TextAlign(ui.End).Padding(t.Space(.5), 0)
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1)).Children(body)
	})
}

func (s *settings) commitSummary(c *ui.Context, entry commitEntry) {
	t := c.Theme()
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(2.5), t.Space(2.5), t.Space(3), 0).
			Gap(t.Space(1.5)).Children(func() {
			ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Start).Gap(t.Space(2.5)).Children(func() {
				authorAvatar(c, entry.Author, t.Space(10.5))
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(.25)).Children(func() {
					components.Text(c, s.text("作者")).FontSize(t.Rem(.7)).TextColor(t.TextMuted)
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
						components.Text(c, entry.Author).FontSize(t.Rem(.88)).Bold().SingleLine()
						components.Text(c, entry.AuthorEmail).FontSize(t.Rem(.8)).TextColor(t.TextMuted).SingleLine()
					})
					components.Text(c, s.dateLabel(entry.Date)).FontSize(t.Rem(.8)).TextColor(t.TextMuted)
				})
			})
			if strings.TrimSpace(entry.Refs) != "" {
				s.detailField(c, s.text("引用"), func() { refPills(c, entry.Refs) })
			}
			s.detailField(c, "SHA", func() {
				components.Text(c, entry.Hash).FontSize(t.Rem(.8)).SingleLine()
			})
			if len(entry.Parents) > 0 {
				s.detailField(c, "PARENTS", func() {
					for _, parent := range entry.Parents {
						short := parent
						if len(short) > 8 {
							short = short[:8]
						}
						components.Text(c, short).FontSize(t.Rem(.8)).TextColor(t.Accent).Shrink(0)
					}
				})
			}
			ui.Box(c).FillWidth().Height(1).Shrink(0).Background(t.Border).Margin(t.Space(1), 0)
			components.Text(c, entry.Subject).Bold().FontSize(t.Rem(.94)).Wrap()
			if entry.Body != "" {
				components.Text(c, entry.Body).Wrap()
			}
		})
	})
}

func (s *settings) commitFileList(c *ui.Context) {
	if !s.contentState(c, &s.repo.files) {
		return
	}
	files := s.repo.files.value
	if len(files) == 0 {
		s.repositoryMessage(c, s.text("没有更改的文件。"))
		return
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Children(func() {
			for _, file := range files {
				s.commitFileRow(c, file)
			}
		})
	})
}

func (s *settings) commitFileTree(c *ui.Context) {
	if !s.contentState(c, &s.repo.files) {
		return
	}
	files := s.repo.files.value
	if len(files) == 0 {
		s.repositoryMessage(c, s.text("没有更改的文件。"))
		return
	}
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Children(func() {
			for _, folder := range commitFileFolders(files) {
				ui.Column(c).Key("folder-" + folder.name).FillWidth().Shrink(0).Children(func() {
					components.TreeItem(c, folder.name, s.groupOpen("commitfile-"+folder.name, true), false, func() {
						for _, file := range folder.files {
							ui.Column(c).Key("file-" + file.Path).FillWidth().Shrink(0).Children(func() {
								components.TreeItem(c, filebase(file.Path), nil, false, nil)
							})
						}
					})
				})
			}
		})
	})
}

func filebase(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		return path[i+1:]
	}
	return path
}

type commitFolder struct {
	name  string
	files []commitFile
}

func commitFileFolders(files []commitFile) []commitFolder {
	var order []string
	groups := map[string][]commitFile{}
	for _, file := range files {
		dir := file.Path
		if i := strings.LastIndexAny(dir, "/\\"); i >= 0 {
			dir = dir[:i]
		} else {
			dir = "."
		}
		if _, ok := groups[dir]; !ok {
			order = append(order, dir)
		}
		groups[dir] = append(groups[dir], file)
	}
	result := make([]commitFolder, 0, len(order))
	for _, name := range order {
		result = append(result, commitFolder{name: name, files: groups[name]})
	}
	return result
}

func (s *settings) commitFileRow(c *ui.Context, file commitFile) {
	t := c.Theme()
	status := file.Status
	if status == "" {
		status = "M"
	}
	path := file.Path
	if file.Orig != "" {
		path = file.Orig + " → " + file.Path
	}
	ui.Row(c).FillWidth().MinWidth(0).Height(t.Space(5.5)).AlignItems(ui.Center).
		Gap(t.Space(1.5)).Padding(0, t.Space(2)).Children(func() {
		components.Text(c, status[:1]).Width(t.Space(3)).Shrink(0).Bold().
			FontSize(t.Rem(.8)).TextColor(changeColor(t, status[:1]))
		components.Icon(c, lucide.File()).Size(t.Space(3.5), t.Space(3.5)).TextColor(t.TextMuted)
		components.Text(c, path).SingleLine().Grow(1).MinWidth(0).FontSize(t.Rem(.82))
	})
}

func (s *settings) filteredCommits(filtered bool) []commitEntry {
	all := s.repo.commits.value
	if !filtered {
		return all
	}
	query := strings.TrimSpace(strings.ToLower(s.repo.query))
	if query == "" {
		return all
	}
	var result []commitEntry
	for _, entry := range all {
		if strings.Contains(strings.ToLower(entry.Subject), query) ||
			strings.Contains(strings.ToLower(entry.Author), query) ||
			strings.Contains(entry.Hash, query) {
			result = append(result, entry)
		}
	}
	return result
}

func (s *settings) branchesView(c *ui.Context) {
	if !s.contentState(c, &s.repo.branches) {
		return
	}
	branches := s.repo.branches.value
	if len(branches) == 0 {
		s.repositoryMessage(c, s.text("没有分支。"))
		return
	}
	components.List(c, &s.repo.branchList, len(branches), func(i int) {
		s.branchRow(c, branches[i])
	}).Fill()
}

func (s *settings) branchRow(c *ui.Context, entry branchEntry) {
	t := c.Theme()
	icon := lucide.GitBranch()
	if entry.Remote {
		icon = lucide.Cloud()
	}
	ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Padding(t.Space(1), t.Space(2.5)).Children(func() {
		components.Icon(c, icon).Size(t.Space(4), t.Space(4)).Shrink(0).TextColor(t.TextMuted)
		components.Text(c, entry.Name).SingleLine().Grow(1).MinWidth(0).Bold()
		if entry.Current {
			components.Text(c, s.text("当前")).FontSize(t.Rem(.8)).TextColor(t.Accent).Shrink(0)
		}
		if entry.Gone {
			components.Text(c, s.text("上游已删除")).FontSize(t.Rem(.8)).TextColor(t.Danger).Shrink(0)
		} else if entry.Upstream != "" {
			track := entry.Upstream
			if entry.Ahead > 0 || entry.Behind > 0 {
				track += fmt.Sprintf(" ↑%d ↓%d", entry.Ahead, entry.Behind)
			}
			components.Text(c, track).FontSize(t.Rem(.8)).TextColor(t.TextMuted).Shrink(0)
		}
		components.Text(c, entry.Subject).SingleLine().FontSize(t.Rem(.8)).TextColor(t.TextMuted).Shrink(0)
	})
}

func (s *settings) searchView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		components.TextBox(c, &s.repo.query, components.TextBoxOptions{LeadingIcon: lucide.Search()}).
			FillWidth().Shrink(0).Padding(t.Space(1.5), t.Space(2)).
			Placeholder(s.text("搜索提交…")).Label(s.text("搜索提交"))
		s.commitsView(c, true)
	})
}
