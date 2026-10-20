package main

import "strings"

func (s *settings) text(zh string) string {
	language := s.values.Language
	if language == "" {
		language = s.systemLanguage
	}
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		return zh
	}
	if en, ok := english[zh]; ok {
		return en
	}
	return zh
}

var english = map[string]string{
	"搜索项目与设置": "Search projects and settings", "搜索项目与设置…": "Search projects and settings…", "搜索…": "Search…", "命令面板": "Command palette", "没有匹配的项目或设置": "No matching projects or settings",
	"正在读取 worktree…": "Loading worktrees…", "读取 worktree 失败：": "Could not load worktrees: ",
	"置顶": "Pinned", "未分组项目": "Ungrouped Projects", "置顶项目": "Pin Project", "取消置顶": "Unpin Project",
	"移到分组": "Move to Group", "新建分组…": "New Group…", "新建分组": "New Group", "重命名分组…": "Rename Group…", "重命名分组": "Rename Group",
	"删除分组": "Delete Group", "分组名称": "Group name", "创建": "Create", "重命名": "Rename", "取消": "Cancel",
	"打开文件夹": "Open Folder", "移除项目": "Remove Project", "恢复已移除的项目": "Restore Removed Projects",
	"无法打开目录": "Could not open folder", "已移除项目": "Project removed", "撤销": "Undo",
	"（已不存在）":        " (missing)",
	"读取状态失败：":       "Could not load state: ",
	"保存状态失败：":       "Could not save state: ",
	"请在设置中选择项目父目录。": "Choose a project parent folder in Settings.",
	"扫描未完成：":        "Scan incomplete: ",
	"未找到 Git 项目。":   "No Git projects found.",
	"Git 项目":        "Git project",
	"从侧边栏选择项目。分支状态与实时更新将在后续实现。": "Select a project in the sidebar. Branch status and live updates are coming later.",
	"语言偏好": "Language preference",
	"设置":   "Settings", "设置…": "Settings…", "关闭设置": "Close settings",
	"主题": "Theme", "主题偏好": "Theme preference", "浅色": "Light", "深色": "Dark", "跟随系统": "System",
	"通用": "General", "应用": "Application", "项目": "Projects", "外观": "Appearance", "语言": "Language",
	"修改后立即生效，并自动保存。": "Changes apply immediately and are saved automatically.",
	"与侧边栏底部的主题选项同步。": "Shared with the theme menu at the bottom of the sidebar.",
	"项目父目录": "Project parent directory", "尚未选择目录": "No directory selected", "选择目录…": "Choose folder…",
	"选择项目父目录": "Choose project parent directory", "选择你常用的项目所在的父目录。": "Choose the parent folder containing your usual projects.", "选择": "Choose",
	"读取设置失败：": "Could not load settings: ", "选择目录失败：": "Could not choose folder: ", "无法访问所选目录：": "Could not access folder: ", "请选择文件夹。": "Please choose a folder.",
	"无法确定设置保存位置。": "Could not determine where to save settings.", "保存失败，设置未更改：%v": "Could not save; settings remain unchanged: %v",
	"搜索设置": "Search settings", "搜索设置…": "Search settings…", "设置分类": "Settings category", "搜索结果": "Search results", "没有匹配的设置": "No matching settings",
	"切换侧边栏": "Toggle sidebar", "调整侧边栏宽度": "Resize sidebar", "尚未接入仓库": "No repositories connected",
	"欢迎使用 Onegit": "Welcome to Onegit", "在一个地方查看多个 Git 仓库与 worktree。": "View multiple Git repositories and worktrees in one place.",
	"应用已初始化。仓库接入、分支状态与实时更新将在后续实现。": "The app is ready. Repository integration, branch status and live updates are coming later.",
}
