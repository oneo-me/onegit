package gitview

import (
	"fmt"
	"strings"
)

// SetLanguage changes the language of the built-in strings, as
// Options.Language chooses it.
func (v *View) SetLanguage(tag string) { v.opts.Language = tag }

// text is the view's string for key in the host's language.
func (v *View) text(key string) string { return v.textf(key, nil) }

// textf is the view's string for key with its {Name} placeholders filled in
// from args. The host's Text answers first; otherwise a Chinese language
// picks the Chinese strings, and a Count of one picks a key's singular form.
func (v *View) textf(key string, args map[string]any) string {
	if v.opts.Text != nil {
		if s := v.opts.Text(key, args); s != "" {
			return s
		}
	}
	table := english
	if strings.HasPrefix(strings.ToLower(v.opts.Language), "zh") {
		table = chinese
	}
	s, ok := table[key]
	if n, counted := args["Count"].(int); counted && n == 1 {
		if one, found := table[key+"_one"]; found {
			s, ok = one, true
		}
	}
	if !ok {
		s = english[key]
	}
	for name, value := range args {
		s = strings.ReplaceAll(s, "{"+name+"}", fmt.Sprint(value))
	}
	return s
}

var english = map[string]string{
	"no_history":              "This project has no Git history to compare",
	"current_changes":         "Current changes",
	"all_commits":             "All commits",
	"branches":                "Branches",
	"remote_branches":         "Remote branches",
	"tags":                    "Tags",
	"filter_refs":             "Filter branches and tags",
	"filter":                  "Filter",
	"no_results":              "No matching results.",
	"no_commits":              "No commits yet",
	"collapse_merge":          "Collapse merged commits",
	"expand_merge":            "Expand merged commits",
	"commit_changes":          "Changes",
	"commit_files":            "Files",
	"commit_message":          "Commit",
	"close_details":           "Close",
	"choose_commit":           "Choose a commit to see its details",
	"no_changes":              "No changes",
	"no_files":                "No files",
	"commit_author":           "Author",
	"commit_committer":        "Committer",
	"commit_co_author":        "Co-author",
	"commit_hash":             "Commit",
	"commit_parents":          "Parents",
	"commit_refs":             "Refs",
	"loading_changes":         "Reading changes…",
	"file_too_large":          "File too large to compare",
	"binary_file":             "Binary file",
	"choose_changed_file":     "Choose a file to see its changes",
	"choose_file":             "Choose a file to see its contents",
	"wrap_lines":              "Wrap lines",
	"image_before":            "Before",
	"image_after":             "After",
	"delete_ref":              "Delete…",
	"delete_branches_title":   "Delete branch {Name}?",
	"delete_branches_message": "Git deletes the branch only when its upstream or the checked out branch has all of its commits.",
	"delete_branches_remote":  "Also delete the remote branch {Target}",
	"delete_remotes_title":    "Delete remote branch {Name}?",
	"delete_remotes_message":  "The branch is deleted from {Remote} for everyone who uses it.",
	"delete_tags_title":       "Delete tag {Name}?",
	"delete_tags_message":     "The commit it points at stays in the history.",
	"delete_tags_remote":      "Also delete the tag from {Remote}",
	"cancel":                  "Cancel",
	"delete":                  "Delete",
	"fetch":                   "Fetch",
	"pull":                    "Pull",
	"push":                    "Push",
	"fetch_failed":            "Fetch failed",
	"pull_failed":             "Pull failed",
	"push_failed":             "Push failed",
	"delete_failed":           "Delete failed",
	"fetch_description":       "Download remote branches and tags without changing your local branches",
	"pull_description":        "Pull remote changes and merge them into your local branch",
	"push_description":        "Push your local changes to a remote repository",
	"no_remotes":              "This repository has no remotes.",
	"sync_remote":             "Remote:",
	"sync_branch":             "Branch:",
	"sync_into":               "Into:",
	"sync_to":                 "To:",
	"fetch_all":               "Fetch all remotes",
	"pull_rebase":             "Rebase instead of merge",
	"pull_stash":              "Stash and reapply local changes",
	"push_default":            "default ({Target})",
	"push_new_branch":         "{Target} (new)",
	"push_tags":               "Push all tags",
	"push_force":              "Force push",
	"fetch_tooltip":           "Update remote branches and tags",
	"pull_tooltip_one":        "Pull {Count} commit from {Upstream} into {Branch}",
	"pull_tooltip":            "Pull {Count} commits from {Upstream} into {Branch}",
	"push_tooltip_one":        "Push {Count} commit of {Branch} to {Upstream}",
	"push_tooltip":            "Push {Count} commits of {Branch} to {Upstream}",
	"publish_tooltip_one":     "Publish {Branch} with {Count} commit no remote has",
	"publish_tooltip":         "Publish {Branch} with {Count} commits no remote has",
	"entry_unavailable":       "This row has nothing to compare",
	"action_failed":           "Git action failed",
}

var chinese = map[string]string{
	"no_history":              "此项目没有可比较的 Git 历史",
	"current_changes":         "当前更改",
	"all_commits":             "全部提交",
	"branches":                "分支",
	"remote_branches":         "远程分支",
	"tags":                    "标签",
	"filter_refs":             "筛选分支和标签",
	"filter":                  "筛选",
	"no_results":              "没有匹配结果。",
	"no_commits":              "尚无提交",
	"collapse_merge":          "折叠合并的提交",
	"expand_merge":            "展开合并的提交",
	"commit_changes":          "更改",
	"commit_files":            "文件树",
	"commit_message":          "提交信息",
	"close_details":           "关闭",
	"choose_commit":           "选择一个提交以查看其详情",
	"no_changes":              "没有更改",
	"no_files":                "没有文件",
	"commit_author":           "作者",
	"commit_committer":        "提交者",
	"commit_co_author":        "合作者",
	"commit_hash":             "提交",
	"commit_parents":          "父提交",
	"commit_refs":             "引用",
	"loading_changes":         "正在读取更改…",
	"file_too_large":          "文件过大，无法比较",
	"binary_file":             "二进制文件",
	"choose_changed_file":     "选择文件以查看其更改",
	"choose_file":             "选择一个文件以查看其内容",
	"wrap_lines":              "自动换行",
	"image_before":            "修改前",
	"image_after":             "修改后",
	"delete_ref":              "删除…",
	"delete_branches_title":   "删除分支 {Name}？",
	"delete_branches_message": "仅当上游分支或当前检出的分支包含其全部提交时，Git 才会删除该分支。",
	"delete_branches_remote":  "同时删除远程分支 {Target}",
	"delete_remotes_title":    "删除远程分支 {Name}？",
	"delete_remotes_message":  "该分支将从 {Remote} 删除，所有使用它的人都会受影响。",
	"delete_tags_title":       "删除标签 {Name}？",
	"delete_tags_message":     "它指向的提交仍保留在历史中。",
	"delete_tags_remote":      "同时删除 {Remote} 上的标签",
	"cancel":                  "取消",
	"delete":                  "删除",
	"fetch":                   "获取",
	"pull":                    "拉取",
	"push":                    "推送",
	"fetch_failed":            "获取失败",
	"pull_failed":             "拉取失败",
	"push_failed":             "推送失败",
	"delete_failed":           "删除失败",
	"fetch_description":       "下载远程分支和标签，不更改本地分支",
	"pull_description":        "拉取远程更改并合并到本地分支",
	"push_description":        "将本地更改推送到远程仓库",
	"no_remotes":              "此仓库没有远程。",
	"sync_remote":             "远程：",
	"sync_branch":             "分支：",
	"sync_into":               "合并到：",
	"sync_to":                 "推送到：",
	"fetch_all":               "从所有远程获取",
	"pull_rebase":             "使用变基代替合并",
	"pull_stash":              "暂存并重新应用本地更改",
	"push_default":            "默认（{Target}）",
	"push_new_branch":         "{Target}（新建）",
	"push_tags":               "推送所有标签",
	"push_force":              "强制推送",
	"fetch_tooltip":           "更新远程分支和标签",
	"pull_tooltip":            "从 {Upstream} 拉取 {Count} 个提交到 {Branch}",
	"push_tooltip":            "将 {Branch} 的 {Count} 个提交推送到 {Upstream}",
	"publish_tooltip":         "发布 {Branch}，含 {Count} 个远程没有的提交",
	"entry_unavailable":       "此行没有可比较的内容",
	"action_failed":           "Git 操作失败",
}
