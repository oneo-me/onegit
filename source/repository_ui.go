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
	switch s.repo.section {
	case sectionCommits, sectionSearch:
		if s.repo.commits.needs(dir) {
			s.repo.commits.start(dispatch, dir, func(ctx context.Context) ([]commitEntry, error) {
				return readCommits(ctx, dir, 300)
			}, s.invalidate)
		}
	case sectionBranches:
		if s.repo.branches.needs(dir) {
			s.repo.branches.start(dispatch, dir, func(ctx context.Context) ([]branchEntry, error) {
				return readBranches(ctx, dir)
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
// navigation actions on the right.
func (s *settings) repositoryToolbar(c *ui.Context) {
	t := c.Theme()
	running := s.repo.job.running
	ui.Row(c).FillWidth().Shrink(0).AlignItems(ui.Center).Gap(t.Space(.5)).
		Padding(t.Space(1.5), t.Space(2), t.Space(1.5), t.Space(2)).Children(func() {
		s.toolbarAction(c, lucide.Download(), s.text("获取"), running, func() {
			s.runGitJob(s.text("正在获取…"), "fetch", "--all", "--prune")
		})
		s.toolbarAction(c, lucide.ArrowDownToLine(), s.text("拉取"), running, func() {
			s.runGitJob(s.text("正在拉取…"), "pull")
		})
		s.toolbarAction(c, lucide.ArrowUpFromLine(), s.text("推送"), running, func() {
			s.runGitJob(s.text("正在推送…"), "push")
		})
		s.toolbarAction(c, lucide.RefreshCw(), s.text("刷新"), running, func() {
			s.refreshRepository()
		})
		ui.Row(c).Grow(1).MinWidth(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			components.Text(c, s.branchStatusLabel()).SingleLine().FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		})
		s.toolbarAction(c, lucide.FolderOpen(), s.text("在文件管理器中显示"), false, func() {
			if s.overview.SelectedPath != "" {
				mygo.Shell.ShowItemInFolder(s.overview.SelectedPath)
			}
		})
	})
}

func (s *settings) toolbarAction(c *ui.Context, icon *ui.SVG, label string, disabled bool, action func()) {
	button := sidebar.IconButton(c, icon, label)
	if disabled {
		button.Disabled(true)
	}
	if button.Clicked() {
		action()
	}
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

func (s *settings) projectSidebar(c *ui.Context) {
	t := c.Theme()
	name := ""
	parent := ""
	if s.selectedProject != nil {
		name = filepath.Base(s.selectedProject.Path)
		parent = filepath.Dir(s.selectedProject.Path)
	}
	ui.Column(c).FillHeight().FillWidth().Background(t.Surface).Clip().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(2.5), t.Space(2), t.Space(2.5), t.Space(1)).Gap(t.Space(.5)).Children(func() {
			components.Text(c, name).Bold().SingleLine()
			components.Text(c, parent).FontSize(t.Rem(.78)).TextColor(t.TextMuted).SingleLine()
		})
		ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(1), t.Space(1), t.Space(1), t.Space(1)).Gap(t.Space(.5)).Children(func() {
			s.sectionItem(c, sectionChanges, s.text("更改"), lucide.FileDiff(), s.changesCountLabel())
			s.sectionItem(c, sectionCommits, s.text("全部提交"), lucide.GitCommitHorizontal(), "")
			s.sectionItem(c, sectionBranches, s.text("分支"), lucide.GitBranch(), "")
			s.sectionItem(c, sectionSearch, s.text("搜索"), lucide.Search(), "")
		})
		ui.Spacer(c)
	})
}

func (s *settings) sectionItem(c *ui.Context, id, label string, icon *ui.SVG, suffix string) {
	item := components.SidebarItem(c, label+suffix, icon, s.repo.section == id).Key(id)
	if item.Clicked() {
		s.setSection(id)
	}
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
		components.List(c, s.commitListState(), len(s.repo.visible), func(i int) {
			s.commitRow(c, s.repo.visible[i])
		}).Fill()
	}, func() {
		s.commitDetails(c, s.repo.visible[s.repo.commitIndex])
	}, components.SplitOptions{Orientation: components.Vertical, FirstMin: 120, SecondMin: 140})
	split.FillWidth().Grow(1).MinHeight(0)
}

func (s *settings) commitListState() *components.ListState {
	list := &s.repo.commitList
	list.Selected = &s.repo.commitIndex
	list.Key = func(i int) any { return s.repo.visible[i].Hash }
	list.Label = func(i int) string { return s.repo.visible[i].Subject }
	list.OnChange = func() {}
	return list
}

func (s *settings) commitRow(c *ui.Context, entry commitEntry) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinWidth(0).AlignItems(ui.Center).Gap(t.Space(1.5)).Padding(t.Space(1), t.Space(2.5)).Children(func() {
		components.Icon(c, lucide.GitCommitHorizontal()).Size(t.Space(4), t.Space(4)).Shrink(0).TextColor(t.TextMuted)
		components.Text(c, entry.Short).FontSize(t.Rem(.8)).TextColor(t.Accent).Shrink(0)
		components.Text(c, entry.Subject).SingleLine().Grow(1).MinWidth(0)
		components.Text(c, entry.Author).FontSize(t.Rem(.8)).TextColor(t.TextMuted).Shrink(0)
		components.Text(c, entry.Date.Format("2006-01-02")).FontSize(t.Rem(.8)).TextColor(t.TextMuted).Shrink(0)
	})
}

func (s *settings) commitDetails(c *ui.Context, entry commitEntry) {
	t := c.Theme()
	components.ScrollView(c, components.ScrollViewOptions{}).Fill().Padding(0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Padding(t.Space(2.5), t.Space(2.5), t.Space(2.5), t.Space(2.5)).Gap(t.Space(1)).Children(func() {
			components.Text(c, entry.Subject).Bold().Wrap()
			components.Text(c, entry.Short).FontSize(t.Rem(.85)).TextColor(t.Accent)
			components.Text(c, entry.Author+" · "+entry.Date.Format("2006-01-02 15:04")).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
			if entry.Refs != "" {
				components.Text(c, entry.Refs).FontSize(t.Rem(.85)).TextColor(t.Accent).Wrap()
			}
			if len(entry.Parents) > 1 {
				components.Text(c, fmt.Sprintf(s.text("合并提交 · %d 个父提交"), len(entry.Parents))).FontSize(t.Rem(.85)).TextColor(t.TextMuted)
			}
			if entry.Body != "" {
				components.Text(c, entry.Body).Wrap()
			}
		})
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
