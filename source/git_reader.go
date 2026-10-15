package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Git overrides from a launcher or development shell must not describe every
// path as the launcher's repository, so reads build a clean environment.
func gitEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return env
}

// gitCommand prepares a Git read. Optional locks stay disabled so background
// reads never block interactive commands.
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(gitEnv(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	return cmd
}

func gitError(dir string, err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		if message := strings.TrimSpace(string(exit.Stderr)); message != "" {
			return fmt.Errorf("%s: %s", dir, message)
		}
	}
	return err
}

// runGitAction runs a Git command that may use the network. Interactive
// prompts stay disabled because the application has no terminal; credential
// helpers still apply.
func runGitAction(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(gitEnv(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s: %s", dir, text)
		}
		return text, fmt.Errorf("%s: %w", dir, err)
	}
	return text, nil
}

// headInfo describes the branch checked out in one worktree.
type headInfo struct {
	Branch   string
	Upstream string
	Ahead    int
	Behind   int
	Detached bool
	Unborn   bool
}

// changeEntry is one path reported by `git status`.
type changeEntry struct {
	Index string
	Work  string
	Path  string
	Orig  string
}

type statusSnapshot struct {
	Head    headInfo
	Changes []changeEntry
}

// readStatus reads the branch header and the working-tree changes together.
func readStatus(ctx context.Context, dir string) (statusSnapshot, error) {
	output, err := gitCommand(ctx, dir, "status", "--porcelain=v1", "-z", "--branch", "--untracked-files=all").Output()
	if err != nil {
		return statusSnapshot{}, gitError(dir, err)
	}
	return parseStatus(string(output))
}

func parseStatus(output string) (statusSnapshot, error) {
	var snapshot statusSnapshot
	tokens := strings.Split(output, "\x00")
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "## ") {
			snapshot.Head = parseBranchHeader(strings.TrimPrefix(token, "## "))
			continue
		}
		if len(token) < 4 {
			return statusSnapshot{}, fmt.Errorf("unrecognized status entry %q", token)
		}
		entry := changeEntry{Index: token[0:1], Work: token[1:2], Path: token[3:]}
		if entry.Index == "R" || entry.Index == "C" || entry.Work == "R" || entry.Work == "C" {
			if i+1 < len(tokens) {
				i++
				entry.Orig = tokens[i]
			}
		}
		snapshot.Changes = append(snapshot.Changes, entry)
	}
	return snapshot, nil
}

func parseBranchHeader(header string) headInfo {
	var info headInfo
	switch {
	case strings.HasPrefix(header, "No commits yet on "):
		info.Unborn = true
		info.Branch = strings.TrimPrefix(header, "No commits yet on ")
		return info
	case strings.HasPrefix(header, "Initial commit on "):
		info.Unborn = true
		info.Branch = strings.TrimPrefix(header, "Initial commit on ")
		return info
	case strings.HasPrefix(header, "HEAD (no branch)"):
		info.Detached = true
		return info
	}
	if open := strings.Index(header, " ["); open >= 0 {
		info.Ahead, info.Behind = parseTrack(header[open+2 : len(header)-1])
		header = header[:open]
	}
	local, upstream, _ := strings.Cut(header, "...")
	info.Branch = local
	info.Upstream = upstream
	return info
}

// parseTrack reads the `[ahead N, behind M]` summary Git appends to a branch line.
func parseTrack(track string) (ahead, behind int) {
	for _, part := range strings.Split(track, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			ahead = value
		case "behind":
			behind = value
		}
	}
	return ahead, behind
}

// branchEntry is one local or remote branch.
type branchEntry struct {
	Name     string
	Hash     string
	Upstream string
	Ahead    int
	Behind   int
	Gone     bool
	Remote   bool
	Current  bool
	Subject  string
}

func readBranches(ctx context.Context, dir string) ([]branchEntry, error) {
	output, err := gitCommand(ctx, dir, "for-each-ref",
		"--format=%(refname)%00%(refname:short)%00%(objectname)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(HEAD)%00%(subject)",
		"refs/heads", "refs/remotes").Output()
	if err != nil {
		return nil, gitError(dir, err)
	}
	return parseBranches(string(output)), nil
}

func parseBranches(output string) []branchEntry {
	var entries []branchEntry
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		for len(fields) < 7 {
			fields = append(fields, "")
		}
		refname := fields[0]
		entry := branchEntry{
			Name:     fields[1],
			Hash:     fields[2],
			Upstream: fields[3],
			Remote:   strings.HasPrefix(refname, "refs/remotes/"),
			Current:  fields[5] == "*",
			Subject:  fields[6],
		}
		entry.Ahead, entry.Behind, entry.Gone = parseBranchTrack(fields[4])
		if strings.HasSuffix(entry.Name, "/HEAD") {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func parseBranchTrack(track string) (ahead, behind int, gone bool) {
	track = strings.TrimSpace(track)
	if track == "" {
		return 0, 0, false
	}
	if strings.Contains(track, "gone") {
		return 0, 0, true
	}
	ahead, behind = parseTrack(track)
	return ahead, behind, false
}

// commitEntry is one revision from `git log`.
type commitEntry struct {
	Hash           string
	Short          string
	Author         string
	AuthorEmail    string
	Committer      string
	CommitterEmail string
	Date           time.Time
	CommitterDate  time.Time
	Parents        []string
	Refs           string
	Subject        string
	Body           string
}

// commitFile is one path touched by a commit, from `git show --name-status`.
type commitFile struct {
	Status string
	Path   string
	Orig   string
}

func readCommits(ctx context.Context, dir string, limit int) ([]commitEntry, error) {
	if limit <= 0 {
		limit = 300
	}
	// --all walks every ref as well as HEAD, so the list covers local and
	// remote branches and tags, not just the checked out worktree.
	// --date-order keeps parents after their children, which the revision
	// graph relies on to close its lines, without interleaving branches the
	// way --topo-order does.
	cmd := gitCommand(ctx, dir, "log",
		"--all",
		"-n", strconv.Itoa(limit),
		"--date-order",
		"--date=iso-strict",
		"--pretty=format:%H%x00%h%x00%an%x00%ae%x00%cn%x00%ce%x00%aI%x00%cI%x00%P%x00%D%x00%s%x00%b%x1e")
	output, err := cmd.Output()
	if err != nil {
		// A repository without commits has no log yet; that is an empty history,
		// not a read failure.
		if exit, ok := err.(*exec.ExitError); ok && strings.Contains(string(exit.Stderr), "does not have any commits yet") {
			return nil, nil
		}
		return nil, gitError(dir, err)
	}
	return parseCommits(string(output)), nil
}

func parseCommits(output string) []commitEntry {
	var entries []commitEntry
	for _, record := range strings.Split(output, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x00")
		for len(fields) < 12 {
			fields = append(fields, "")
		}
		date, _ := time.Parse(time.RFC3339, fields[6])
		commitDate, _ := time.Parse(time.RFC3339, fields[7])
		entry := commitEntry{
			Hash:           fields[0],
			Short:          fields[1],
			Author:         fields[2],
			AuthorEmail:    fields[3],
			Committer:      fields[4],
			CommitterEmail: fields[5],
			Date:           date,
			CommitterDate:  commitDate,
			Refs:           fields[9],
			Subject:        fields[10],
			Body:           strings.TrimRight(fields[11], "\n"),
		}
		if parents := strings.Fields(fields[8]); len(parents) > 0 {
			entry.Parents = parents
		}
		entries = append(entries, entry)
	}
	return entries
}

// readCommitFiles lists the paths a commit changed, used by the Changes and
// File Tree tabs of the commit details.
func readCommitFiles(ctx context.Context, dir, hash string) ([]commitFile, error) {
	if hash == "" {
		return nil, nil
	}
	output, err := gitCommand(ctx, dir, "show", "--name-status", "--format=", "-z", hash).Output()
	if err != nil {
		return nil, gitError(dir, err)
	}
	return parseCommitFiles(string(output)), nil
}

func parseCommitFiles(output string) []commitFile {
	var files []commitFile
	tokens := strings.Split(output, "\x00")
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "" {
			continue
		}
		parts := strings.SplitN(token, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status, path := parts[0], parts[1]
		entry := commitFile{Status: status, Path: path}
		if status != "" && (status[0] == 'R' || status[0] == 'C') {
			entry.Orig = path
			if i+1 < len(tokens) {
				i++
				entry.Path = tokens[i]
			}
		}
		files = append(files, entry)
	}
	return files
}

// refEntry is one tag, stash or submodule row of the repository sidebar, with
// the commit its row reveals on the right when selected. Submodules leave Hash
// empty because the recorded commit lives in the submodule's own repository.
type refEntry struct {
	Name string
	Hash string
}

// sidebarRefs groups the tag, stash and submodule rows that the repository
// sidebar lists next to the branches.
type sidebarRefs struct {
	Tags       []refEntry
	Stashes    []refEntry
	Submodules []refEntry
}

func readSidebarRefs(ctx context.Context, dir string) (sidebarRefs, error) {
	var refs sidebarRefs
	// An annotated tag names a tag object; its peeled object is the commit the
	// tag points at, which is what selecting the row should reveal.
	if output, err := gitCommand(ctx, dir, "for-each-ref",
		"--format=%(refname:short)%00%(objectname)%00%(*objectname)",
		"--sort=-creatordate", "refs/tags").Output(); err == nil {
		for _, line := range splitLines(string(output)) {
			fields := strings.Split(line, "\x00")
			entry := refEntry{Name: fields[0]}
			if len(fields) > 1 {
				entry.Hash = fields[1]
			}
			if len(fields) > 2 && fields[2] != "" {
				entry.Hash = fields[2]
			}
			refs.Tags = append(refs.Tags, entry)
		}
	}
	if output, err := gitCommand(ctx, dir, "stash", "list", "--format=%gd%x00%H%x00%s").Output(); err == nil {
		for _, line := range splitLines(string(output)) {
			fields := strings.SplitN(line, "\x00", 3)
			entry := refEntry{Name: fields[0]}
			if len(fields) > 1 {
				entry.Hash = fields[1]
			}
			if len(fields) > 2 && fields[2] != "" {
				entry.Name += " " + fields[2]
			}
			refs.Stashes = append(refs.Stashes, entry)
		}
	}
	if output, err := gitCommand(ctx, dir, "submodule", "status").Output(); err == nil {
		for _, line := range splitLines(string(output)) {
			fields := strings.Fields(strings.TrimLeft(line, " +-U"))
			if len(fields) > 0 {
				refs.Submodules = append(refs.Submodules, refEntry{Name: fields[0]})
			}
		}
	}
	return refs, nil
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
