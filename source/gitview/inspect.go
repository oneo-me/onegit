package gitview

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// commitFormat is the pretty format parseCommits reads: hash, short hash,
// parents, decorations, author, author email, author date, committer,
// committer email, commit date, co-authors, subject and body.
const commitFormat = "%H%x1f%h%x1f%P%x1f%D%x1f%an%x1f%ae%x1f%aI%x1f%cn%x1f%ce%x1f%cI%x1f%(trailers:key=Co-authored-by,valueonly,unfold,separator=%x1e)%x1f%s%x1f%b"

// Reflog reads the commits the repository containing path's reflogs name,
// newest first and without repeats, so commits no ref reaches still show.
func (g Git) Reflog(path string) ([]Commit, error) {
	out, err := g.output(historyTimeout, path, "--no-optional-locks", "log", "-g", "--all", "-z",
		"--pretty=format:"+commitFormat)
	if err != nil {
		return nil, err
	}
	return uniqueCommits(parseCommits(string(out))), nil
}

// FileHistory reads the commits, newest first, that changed file in the
// history of the checked out branch, following renames.
func (g Git) FileHistory(path, file string) ([]Commit, error) {
	out, err := g.output(historyTimeout, path, "--no-optional-locks", "log", "--follow", "--date-order", "-z",
		"--pretty=format:"+commitFormat, "--", file)
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

// SearchType is what a commit search matches.
type SearchType int

const (
	SearchAll SearchType = iota
	SearchMessage
	SearchAuthor
	SearchPath
	SearchContent
)

// SearchScope is where a commit search looks.
type SearchScope int

const (
	SearchRepository SearchScope = iota
	SearchCurrentBranch
)

// SearchOptions choose what a commit search matches and where it looks.
type SearchOptions struct {
	Type  SearchType
	Scope SearchScope
	Query string
}

// SearchCommits reads the commits whose message, author, changed paths or
// changed content contain the query, ignoring case. The repository scope
// covers every branch, remote branch and tag; the current branch scope
// covers the checked out branch's history.
func (g Git) SearchCommits(path string, o SearchOptions) ([]Commit, error) {
	query := strings.TrimSpace(o.Query)
	if query == "" {
		return nil, nil
	}
	types := []SearchType{SearchMessage, SearchAuthor, SearchPath, SearchContent}
	if o.Type != SearchAll {
		types = []SearchType{o.Type}
	}
	seen := map[string]bool{}
	var commits []Commit
	for _, typ := range types {
		found, err := g.searchCommits(path, o.Scope, typ, query)
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			if !seen[c.Hash] {
				seen[c.Hash] = true
				commits = append(commits, c)
			}
		}
	}
	slices.SortStableFunc(commits, func(a, b Commit) int { return b.CommitDate.Compare(a.CommitDate) })
	return commits, nil
}

func (g Git) searchCommits(path string, scope SearchScope, typ SearchType, query string) ([]Commit, error) {
	args := []string{"--no-optional-locks", "log", "--date-order", "-z", "--pretty=format:" + commitFormat}
	if scope == SearchRepository {
		args = append(args, "--all")
	}
	escaped := regexp.QuoteMeta(query)
	switch typ {
	case SearchMessage:
		args = append(args, "--regexp-ignore-case", "--grep="+escaped)
	case SearchAuthor:
		args = append(args, "--regexp-ignore-case", "--author="+escaped)
	case SearchPath:
		args = append(args, "--", "*"+query+"*")
	case SearchContent:
		args = append(args, "--regexp-ignore-case", "-G"+escaped)
	default:
		return nil, nil
	}
	out, err := g.output(historyTimeout, path, args...)
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

func uniqueCommits(commits []Commit) []Commit {
	seen := make(map[string]bool, len(commits))
	out := commits[:0]
	for _, c := range commits {
		if !seen[c.Hash] {
			seen[c.Hash] = true
			out = append(out, c)
		}
	}
	return out
}

// BlameLine is one line of a file's blame: the commit that last changed it
// and the line's text.
type BlameLine struct {
	Hash, Short         string
	Author, AuthorEmail string
	Date                time.Time
	Text                string
}

// Blame attributes each line of file, relative to the worktree at path, to
// the commit that last changed it. An empty rev blames the worktree's
// version; otherwise rev names the commit to blame.
func (g Git) Blame(path, rev, file string) ([]BlameLine, error) {
	args := []string{"--no-optional-locks", "blame", "--line-porcelain"}
	if rev != "" {
		args = append(args, rev)
	}
	args = append(args, "--", file)
	out, err := g.output(readTimeout, path, args...)
	if err != nil {
		return nil, err
	}
	var lines []BlameLine
	var current BlameLine
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "\t") {
			current.Text = strings.TrimPrefix(line, "\t")
			lines = append(lines, current)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 4 && isHash(fields[0]) {
			current = BlameLine{Hash: fields[0], Short: shortHash(fields[0])}
			continue
		}
		switch {
		case strings.HasPrefix(line, "author "):
			current.Author = strings.TrimPrefix(line, "author ")
		case strings.HasPrefix(line, "author-mail "):
			current.AuthorEmail = strings.Trim(strings.TrimPrefix(line, "author-mail "), "<>")
		case strings.HasPrefix(line, "author-time "):
			if n, err := strconv.ParseInt(strings.TrimPrefix(line, "author-time "), 10, 64); err == nil {
				current.Date = time.Unix(n, 0)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func isHash(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// DiffBetween lists the files from and to differ in, sorted by path, or
// those from differs in from the worktree when to is empty, untracked files
// included.
func (g Git) DiffBetween(path, from, to string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	if to == "" {
		return g.diffWorktree(top, from)
	}
	changes := Changes{Top: top, From: from, To: to}
	if changes.Files, err = g.diffFiles(top, "diff", from, to); err != nil {
		return Changes{}, err
	}
	slices.SortFunc(changes.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return changes, nil
}

// diffWorktree lists the files the worktree at top differs in from commit,
// including untracked files as additions.
func (g Git) diffWorktree(top, commit string) (Changes, error) {
	changes := Changes{Top: top, From: commit}
	files, err := g.diffFiles(top, "diff", commit)
	if err != nil {
		return Changes{}, err
	}
	untracked, err := g.Run(top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	for name := range strings.SplitSeq(untracked, "\x00") {
		if name != "" {
			files = append(files, FileChange{Path: name, OldPath: name, Status: 'A',
				Added: g.textLines(filepath.Join(top, filepath.FromSlash(name)))})
		}
	}
	slices.SortFunc(files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	changes.Files = files
	return changes, nil
}
