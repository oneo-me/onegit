package gitview

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Timeouts bound local reads, full history reads and commands that may reach
// a remote.
const (
	readTimeout    = 60 * time.Second
	historyTimeout = 5 * time.Minute
	networkTimeout = 5 * time.Minute
)

// ErrDetached is a push that names no local branch.
var ErrDetached = errors.New("no branch is checked out")

// ErrNoRemote is a push that names no remote to send the branch to.
var ErrNoRemote = errors.New("no remote to push the branch to")

// Git runs the git executable with Env, or the process environment when Env
// is nil. When Env sets PATH, git is looked up there. With Host, git runs and
// files are read there instead, and Env only adds to the host's environment.
type Git struct {
	Env  []string
	Host Host
}

// Run runs git in dir and returns its trimmed standard output, or its
// standard error as the error.
func (g Git) Run(dir string, args ...string) (string, error) {
	return g.runFor(readTimeout, dir, args...)
}

func (g Git) runFor(timeout time.Duration, dir string, args ...string) (string, error) {
	out, err := g.output(timeout, dir, args...)
	return strings.TrimSpace(string(out)), err
}

// output runs git and returns its standard output untrimmed.
func (g Git) output(timeout time.Duration, dir string, args ...string) ([]byte, error) {
	return g.outputWith(timeout, dir, nil, args...)
}

// outputWith runs git with input as its standard input, unless it is nil.
func (g Git) outputWith(timeout time.Duration, dir string, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if g.Host != nil {
		stdout, stderr, err := g.Host.Run(ctx, dir, g.Env, input, args...)
		if err != nil {
			if msg := strings.TrimSpace(string(stderr)); msg != "" {
				return nil, errors.New(msg)
			}
			return nil, err
		}
		return stdout, nil
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	hideConsole(cmd)
	cmd.Dir = dir
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	if g.Env != nil {
		cmd.Env = g.Env
		if path := lookGit(g.Env); path != "" {
			cmd.Path = path
		}
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func lookGit(env []string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(env[i], "="); ok && k == "PATH" {
			for _, dir := range filepath.SplitList(v) {
				p := filepath.Join(dir, "git")
				if info, err := os.Stat(p); err == nil && !info.IsDir() {
					return p
				}
			}
			return ""
		}
	}
	return ""
}

func (g Git) withEnv(values ...string) Git {
	env := g.Env
	if env == nil && g.Host == nil {
		env = os.Environ()
	}
	g.Env = append(append([]string(nil), env...), values...)
	return g
}

// network runs a command that may reach a remote. It never prompts, so a
// missing credential fails instead of hanging.
func (g Git) network(path string, args ...string) error {
	_, err := g.withEnv("GIT_TERMINAL_PROMPT=0").runFor(networkTimeout, path, args...)
	return err
}

// Top returns the top of the worktree containing path, failing outside a
// repository.
func (g Git) Top(path string) (string, error) {
	return g.Run(path, "rev-parse", "--show-toplevel")
}

// Unborn reports whether the worktree at path has no commit checked out yet.
func (g Git) Unborn(path string) bool {
	_, err := g.Run(path, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return err != nil
}

// Commit is one revision of a repository's history.
type Commit struct {
	Hash, Short string
	// Parents lists the parent hashes, the first parent first.
	Parents []string
	// Refs are the branches, remote branches and tags pointing at the
	// commit, as git log decorates it: "HEAD -> main", "origin/main" or
	// "tag: v1".
	Refs                      []string
	Author, AuthorEmail       string
	Committer, CommitterEmail string
	AuthorDate, CommitDate    time.Time
	// CoAuthors are the people the message's Co-authored-by trailers name,
	// in order, without repeats.
	CoAuthors     []Person
	Subject, Body string
}

// Person is a name and an email address, either of which may be empty.
type Person struct {
	Name, Email string
}

// Commits reads the history of the repository containing path, newest first
// with each commit after its children so a revision graph can follow it. It
// covers every branch, remote branch and tag but the hidden ones, the
// checked out commit and tips. A repository without commits has an empty
// history.
func (g Git) Commits(path string, hidden Refs, tips ...string) ([]Commit, error) {
	args := []string{"--no-optional-locks", "log", "--date-order", "-z",
		"--decorate-refs=refs/heads", "--decorate-refs=refs/remotes", "--decorate-refs=refs/tags", "--decorate-refs=HEAD",
		"--pretty=format:%H%x1f%h%x1f%P%x1f%D%x1f%an%x1f%ae%x1f%aI%x1f%cn%x1f%ce%x1f%cI%x1f%(trailers:key=Co-authored-by,valueonly,unfold,separator=%x1e)%x1f%s%x1f%b"}
	for _, group := range []struct {
		refs   []Ref
		prefix string
		all    string
	}{{hidden.Branches, "refs/heads/", "--branches"}, {hidden.Remotes, "refs/remotes/", "--remotes"}, {hidden.Tags, "refs/tags/", "--tags"}} {
		for _, r := range group.refs {
			args = append(args, "--decorate-refs-exclude="+group.prefix+globEscape(r.Name), "--exclude="+globEscape(r.Name))
		}
		args = append(args, group.all)
	}
	args = append(args, tips...)
	if !g.Unborn(path) {
		args = append(args, "HEAD")
	} else if refs, _ := g.Run(path, "for-each-ref", "--count=1", "refs/heads", "refs/remotes", "refs/tags"); refs == "" {
		return nil, nil
	}
	out, err := g.output(historyTimeout, path, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

// globEscape quotes the characters a Git ref pattern treats specially, so
// the pattern matches name alone.
func globEscape(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`*?[\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func parseCommits(out string) []Commit {
	var commits []Commit
	for record := range strings.SplitSeq(out, "\x00") {
		fields := strings.Split(strings.TrimLeft(record, "\n"), "\x1f")
		if len(fields) < 13 || fields[0] == "" {
			continue
		}
		authored, _ := time.Parse(time.RFC3339, fields[6])
		committed, _ := time.Parse(time.RFC3339, fields[9])
		c := Commit{
			Hash: fields[0], Short: fields[1], Parents: strings.Fields(fields[2]),
			Author: fields[4], AuthorEmail: fields[5], AuthorDate: authored,
			Committer: fields[7], CommitterEmail: fields[8], CommitDate: committed,
			CoAuthors: parseCoAuthors(fields[10]),
			Subject:   fields[11], Body: strings.TrimSpace(strings.Join(fields[12:], "\x1f")),
		}
		for ref := range strings.SplitSeq(fields[3], ", ") {
			if ref != "" {
				c.Refs = append(c.Refs, ref)
			}
		}
		commits = append(commits, c)
	}
	return commits
}

// parseCoAuthors reads trailer values such as "Name <email>", separated by
// U+001E, skipping repeated addresses.
func parseCoAuthors(values string) []Person {
	var people []Person
	for value := range strings.SplitSeq(values, "\x1e") {
		var p Person
		value = strings.TrimSpace(value)
		if name, rest, ok := strings.Cut(value, "<"); ok {
			p = Person{Name: strings.TrimSpace(name), Email: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), ">"))}
		} else if strings.Contains(value, "@") {
			p.Email = value
		} else {
			p.Name = value
		}
		if p == (Person{}) || slices.ContainsFunc(people, func(o Person) bool {
			return strings.EqualFold(o.Email, p.Email) && (p.Email != "" || o.Name == p.Name)
		}) {
			continue
		}
		people = append(people, p)
	}
	return people
}

// RefKind is the group of a ref: a local branch, remote branch or tag.
type RefKind int

const (
	RefBranch RefKind = iota
	RefRemote
	RefTag
)

// Ref is a branch, remote branch or tag and the commit it points at. A
// local branch also names the branch of a remote it tracks while that
// exists.
type Ref struct {
	Name, Hash string
	Upstream   RemoteBranch
}

// Refs are a repository's local branches, remote branches and tags, each
// sorted by name.
type Refs struct {
	Branches, Remotes, Tags []Ref
}

// split keeps the refs keep accepts, returning them and the rest. A nil keep
// keeps every ref.
func (r Refs) split(keep func(RefKind, string) bool) (kept, hidden Refs) {
	if keep == nil {
		return r, Refs{}
	}
	part := func(kind RefKind, refs []Ref, kept, hidden *[]Ref) {
		for _, ref := range refs {
			if keep(kind, ref.Name) {
				*kept = append(*kept, ref)
			} else {
				*hidden = append(*hidden, ref)
			}
		}
	}
	part(RefBranch, r.Branches, &kept.Branches, &hidden.Branches)
	part(RefRemote, r.Remotes, &kept.Remotes, &hidden.Remotes)
	part(RefTag, r.Tags, &kept.Tags, &hidden.Tags)
	return kept, hidden
}

// Refs reads the branches, remote branches and tags of the repository
// containing path. A tag names the commit it points at, peeled through an
// annotated tag; a remote's HEAD alias is left out.
func (g Git) Refs(path string) (Refs, error) {
	out, err := g.Run(path, "for-each-ref",
		"--format=%(refname)%00%(objectname)%00%(*objectname)%00%(upstream:remotename)%00%(upstream:remoteref)%00%(upstream:track)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return Refs{}, err
	}
	var refs Refs
	for line := range strings.Lines(out) {
		fields := strings.Split(strings.TrimRight(line, "\n"), "\x00")
		if len(fields) < 6 {
			continue
		}
		ref := Ref{Hash: fields[1]}
		if fields[2] != "" {
			ref.Hash = fields[2]
		}
		var ok bool
		switch {
		case strings.HasPrefix(fields[0], "refs/heads/"):
			ref.Name = strings.TrimPrefix(fields[0], "refs/heads/")
			if branch, ok := strings.CutPrefix(fields[4], "refs/heads/"); ok && fields[3] != "" && fields[3] != "." && fields[5] != "[gone]" {
				ref.Upstream = RemoteBranch{fields[3], branch}
			}
			refs.Branches = append(refs.Branches, ref)
		case strings.HasPrefix(fields[0], "refs/remotes/"):
			if ref.Name, ok = strings.CutPrefix(fields[0], "refs/remotes/"); ok && !strings.HasSuffix(ref.Name, "/HEAD") {
				refs.Remotes = append(refs.Remotes, ref)
			}
		case strings.HasPrefix(fields[0], "refs/tags/"):
			ref.Name = strings.TrimPrefix(fields[0], "refs/tags/")
			refs.Tags = append(refs.Tags, ref)
		}
	}
	return refs, nil
}

// GitHubRemotes maps every remote of the repository containing path to
// whether its fetch URL is on github.com, over HTTPS or SSH.
func (g Git) GitHubRemotes(path string) (map[string]bool, error) {
	out, err := g.Run(path, "remote", "-v")
	if err != nil {
		return nil, err
	}
	remotes := map[string]bool{}
	for line := range strings.Lines(out) {
		name, rest, _ := strings.Cut(strings.TrimSpace(line), "\t")
		url, kind, _ := strings.Cut(rest, " ")
		if kind == "(fetch)" {
			remotes[name] = onGitHub(url)
		}
	}
	return remotes, nil
}

// onGitHub reports whether a remote URL, such as https://github.com/o/r,
// git@github.com:o/r or ssh://git@github.com/o/r, is on github.com.
func onGitHub(url string) bool {
	if _, rest, ok := strings.Cut(url, "://"); ok {
		url = rest
	} else if !strings.Contains(url, ":") {
		// A path without a scheme or host is a local repository.
		return false
	}
	host, _, _ := strings.Cut(url, "/")
	host, _, _ = strings.Cut(host, ":")
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	return strings.EqualFold(host, "github.com") || strings.EqualFold(host, "www.github.com")
}

// Checkout is one of a repository's worktrees.
type Checkout struct {
	// Path is the worktree's top; Branch its checked out branch, empty for
	// a detached HEAD; Head the commit it has checked out.
	Path, Branch, Head string
}

// Checkouts lists the worktrees of the repository containing path, its main
// worktree first, leaving out bare and pruned ones.
func (g Git) Checkouts(path string) ([]Checkout, error) {
	out, err := g.Run(path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var list []Checkout
	var cur Checkout
	skip := false
	flush := func() {
		if cur.Path != "" && !skip {
			list = append(list, cur)
		}
		cur, skip = Checkout{}, false
	}
	for field := range strings.SplitSeq(out, "\x00") {
		switch key, value, _ := strings.Cut(field, " "); key {
		case "":
			flush()
		case "worktree":
			cur.Path = value
		case "HEAD":
			cur.Head = value
		case "branch":
			cur.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare", "prunable":
			skip = true
		}
	}
	flush()
	return list, nil
}

// FileChange is a file a worktree or commit changes.
type FileChange struct {
	// Path is the file's path relative to the worktree's top, with forward
	// slashes; OldPath is its earlier path, which differs only for renames.
	Path, OldPath string
	// Status is Git's letter: A added, including untracked files, M modified,
	// D deleted, R renamed or T changed in type.
	Status byte
	// Added and Deleted count changed lines; both are zero for a binary
	// file.
	Added, Deleted int
	// Area tells a worktree's staged changes from its unstaged ones.
	Area Area
}

// Area is where a worktree's change lies.
type Area uint8

const (
	// AreaAll compares the worktree or a commit with Changes.From,
	// whatever the index holds.
	AreaAll Area = iota
	// AreaStaged compares the index with Changes.From.
	AreaStaged
	// AreaUnstaged compares the worktree with the index.
	AreaUnstaged
)

// Changes are the files a worktree changes since a commit, or a commit
// changes since its first parent, or the files a commit or worktree holds.
type Changes struct {
	// Top is the worktree's top and From the commit the files compare with,
	// empty before a root commit. To is the commit holding the files' new
	// versions, or empty for the worktree's files.
	Top, From, To string
	Files         []FileChange
}

// WorktreeChanges lists the files the worktree containing path changes since
// it forked from base, including uncommitted and untracked ones, sorted by
// path. Without a base it lists only uncommitted work.
func (g Git) WorktreeChanges(path, base string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	from := "HEAD"
	if base != "" {
		if from, err = g.Run(top, "merge-base", base, "HEAD"); err != nil {
			return Changes{}, err
		}
	}
	// Pin the commit so the files read later match this list.
	if from, err = g.Run(top, "rev-parse", "--verify", from+"^{commit}"); err != nil {
		return Changes{}, err
	}
	changes := Changes{Top: top, From: from}
	if changes.Files, err = g.diffFiles(top, "diff", from); err != nil {
		return Changes{}, err
	}
	untracked, err := g.Run(top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	for name := range strings.SplitSeq(untracked, "\x00") {
		if name != "" {
			changes.Files = append(changes.Files, FileChange{Path: name, OldPath: name, Status: 'A',
				Added: g.textLines(filepath.Join(top, filepath.FromSlash(name)))})
		}
	}
	slices.SortFunc(changes.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return changes, nil
}

// WorktreeStatus lists the changes the index of the worktree containing path
// stages since its checked out commit, then those of the worktree since the
// index, including untracked files, each sorted by path. Before the first
// commit, every staged file is an addition and From is empty.
func (g Git) WorktreeStatus(path string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	changes := Changes{Top: top}
	staged := []string{"diff", "--cached"}
	if !g.Unborn(top) {
		// Pin the commit so the files read later match this list.
		if changes.From, err = g.Run(top, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
			return Changes{}, err
		}
		staged = append(staged, changes.From)
	}
	index, err := g.diffFiles(top, staged...)
	if err != nil {
		return Changes{}, err
	}
	for _, f := range index {
		// A conflicted file has no single staged version; it lists as
		// unstaged.
		if f.Status != 'U' {
			f.Area = AreaStaged
			changes.Files = append(changes.Files, f)
		}
	}
	worktree, err := g.diffFiles(top, "diff")
	if err != nil {
		return Changes{}, err
	}
	untracked, err := g.Run(top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	for name := range strings.SplitSeq(untracked, "\x00") {
		if name != "" {
			worktree = append(worktree, FileChange{Path: name, OldPath: name, Status: 'A',
				Added: g.textLines(filepath.Join(top, filepath.FromSlash(name)))})
		}
	}
	slices.SortFunc(changes.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(worktree, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	for _, f := range worktree {
		f.Area = AreaUnstaged
		changes.Files = append(changes.Files, f)
	}
	return changes, nil
}

// diffFiles lists the files a diff command reports, with their changed
// lines. diff is the command and its revisions, as "diff" and a commit.
func (g Git) diffFiles(top string, diff ...string) ([]FileChange, error) {
	args := append([]string{"--no-optional-locks"}, diff[0], "--name-status", "-z", "-M")
	out, err := g.output(readTimeout, top, append(args, diff[1:]...)...)
	if err != nil {
		return nil, err
	}
	var files []FileChange
	fields := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(fields); {
		status := fields[i]
		if status == "" {
			break
		}
		f := FileChange{Status: status[0], Path: fields[i+1], OldPath: fields[i+1]}
		i += 2
		if (f.Status == 'R' || f.Status == 'C') && i < len(fields) {
			f.Path = fields[i]
			i++
		}
		if f.Status == 'C' {
			// A copy leaves its source in place, so it shows as an addition.
			f.Status, f.OldPath = 'A', f.Path
		}
		files = append(files, f)
	}
	return files, g.countLines(top, files, diff)
}

// countLines fills in the changed lines of files, as the diff command reports.
func (g Git) countLines(top string, files []FileChange, diff []string) error {
	args := append([]string{"--no-optional-locks"}, diff[0], "--numstat", "-z", "-M")
	out, err := g.output(readTimeout, top, append(args, diff[1:]...)...)
	if err != nil {
		return err
	}
	type counts struct{ added, deleted int }
	lines := map[string]counts{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields) && fields[i] != ""; i++ {
		parts := strings.SplitN(fields[i], "\t", 3)
		if len(parts) != 3 {
			break
		}
		path := parts[2]
		if path == "" && i+2 < len(fields) {
			// A rename lists its old path, then its new one.
			path = fields[i+2]
			i += 2
		}
		// Binary files report "-", which counts as zero.
		added, _ := strconv.Atoi(parts[0])
		deleted, _ := strconv.Atoi(parts[1])
		lines[path] = counts{added, deleted}
	}
	for i := range files {
		c := lines[files[i].Path]
		files[i].Added, files[i].Deleted = c.added, c.deleted
	}
	return nil
}

// textLines counts the lines of a regular file as Git's numstat would, or
// returns 0 for unreadable files and those Git treats as binary.
func (g Git) textLines(path string) int {
	if info, err := g.lstat(path); err != nil || !info.Mode().IsRegular() {
		return 0
	}
	data, err := g.readFile(path)
	if err != nil || isBinary(data) {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// isBinary reports a file Git would treat as binary.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0
}

// FileVersions reads a changed file at the commit changes compare with and
// in the worktree, or at the commit changes are of. A staged change reads
// its new version from the index, and an unstaged one its old version. The
// side a change lacks, before an addition or after a deletion, is nil. A
// symbolic link reads as its target, as Git stores it.
func (g Git) FileVersions(changes Changes, f FileChange) (before, after []byte, err error) {
	if f.Status != 'A' {
		if f.Area == AreaUnstaged {
			before, err = g.IndexFile(changes.Top, f.OldPath)
		} else {
			before, err = g.FileAt(changes.Top, changes.From, f.OldPath)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	if f.Status == 'D' {
		return before, nil, nil
	}
	if f.Area == AreaStaged {
		after, err = g.IndexFile(changes.Top, f.Path)
		return before, after, err
	}
	if changes.To != "" {
		after, err = g.FileAt(changes.Top, changes.To, f.Path)
		return before, after, err
	}
	name := filepath.Join(changes.Top, filepath.FromSlash(f.Path))
	info, err := g.lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := g.readlink(name)
		return before, []byte(filepath.ToSlash(target)), err
	}
	after, err = g.readFile(name)
	return before, after, err
}

// FileAt reads the file at path, relative to the repository's top, in commit.
func (g Git) FileAt(top, commit, path string) ([]byte, error) {
	return g.output(readTimeout, top, "cat-file", "blob", commit+":"+path)
}

// IndexFile reads the file at path, relative to the repository's top, as
// the index stages it.
func (g Git) IndexFile(top, path string) ([]byte, error) {
	return g.output(readTimeout, top, "cat-file", "blob", ":"+path)
}

// pathsOf lists the paths files change, both of a rename's.
func pathsOf(files []FileChange) []string {
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
		if f.OldPath != "" && f.OldPath != f.Path {
			paths = append(paths, f.OldPath)
		}
	}
	return paths
}

// Stage stages the worktree's versions of files in the worktree at top,
// their deletion included.
func (g Git) Stage(top string, files ...FileChange) error {
	_, err := g.Run(top, append([]string{"--literal-pathspecs", "add", "--all", "--"}, pathsOf(files)...)...)
	return err
}

// Unstage puts the checked out commit's versions of files back in the index
// of the worktree at top, leaving the worktree as it is. Before the first
// commit, it removes them from the index.
func (g Git) Unstage(top string, files ...FileChange) error {
	args := []string{"--literal-pathspecs", "restore", "--staged", "--"}
	if g.Unborn(top) {
		args = []string{"--literal-pathspecs", "rm", "--cached", "--quiet", "-r", "--"}
	}
	_, err := g.Run(top, append(args, pathsOf(files)...)...)
	return err
}

// StageText stages text as the version of the file at path, relative to the
// worktree's top, keeping the mode the index or checked out commit gives it,
// else the worktree file's.
func (g Git) StageText(top, path string, text []byte) error {
	mode, err := g.fileMode(top, path)
	if err != nil {
		return err
	}
	blob, err := g.outputWith(readTimeout, top, text, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	_, err = g.Run(top, "update-index", "--add", "--cacheinfo", mode+","+strings.TrimSpace(string(blob))+","+path)
	return err
}

func (g Git) fileMode(top, path string) (string, error) {
	out, err := g.Run(top, "--literal-pathspecs", "ls-files", "--stage", "--", path)
	if err != nil {
		return "", err
	}
	if mode, _, ok := strings.Cut(out, " "); ok {
		return mode, nil
	}
	if !g.Unborn(top) {
		if out, err = g.Run(top, "--literal-pathspecs", "ls-tree", "HEAD", "--", path); err != nil {
			return "", err
		}
		if mode, _, ok := strings.Cut(out, " "); ok {
			return mode, nil
		}
	}
	if info, err := g.stat(filepath.Join(top, filepath.FromSlash(path))); err == nil && info.Mode()&0o111 != 0 {
		return "100755", nil
	}
	return "100644", nil
}

// Commit records what the index of the worktree at top stages as a new
// commit with message, cleaned up as Git's editor would.
func (g Git) Commit(top, message string) error {
	_, err := g.outputWith(networkTimeout, top, []byte(message), "commit", "--quiet", "--cleanup=strip", "--file=-")
	return err
}

// CommitChanges lists the files a commit changes since its first parent, or
// adds as a root commit, sorted by path. It runs in the repository
// containing path.
func (g Git) CommitChanges(path, hash string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	changes := Changes{Top: top, To: hash}
	diff := []string{"diff-tree", "-r", "--root", "--no-commit-id", hash}
	if parent, err := g.Run(top, "rev-parse", "--verify", "--quiet", hash+"^1"); err == nil && parent != "" {
		changes.From = parent
		diff = []string{"diff", parent, hash}
	}
	if changes.Files, err = g.diffFiles(top, diff...); err != nil {
		return Changes{}, err
	}
	slices.SortFunc(changes.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return changes, nil
}

// CommitFiles lists every file of a commit, sorted by path, as additions
// whose versions FileVersions reads from the commit. It runs in the
// repository containing path.
func (g Git) CommitFiles(path, hash string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	out, err := g.output(readTimeout, top, "ls-tree", "-r", "--name-only", "-z", hash)
	if err != nil {
		return Changes{}, err
	}
	files := Changes{Top: top, To: hash}
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name != "" {
			files.Files = append(files.Files, FileChange{Path: name, OldPath: name, Status: 'A'})
		}
	}
	slices.SortFunc(files.Files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

// WorktreeFiles lists every file of the worktree containing path, tracked or
// untracked but not ignored, sorted by path, as additions whose versions
// FileVersions reads from the worktree. Deleted files and submodules stay
// out.
func (g Git) WorktreeFiles(path string) (Changes, error) {
	top, err := g.Top(path)
	if err != nil {
		return Changes{}, err
	}
	out, err := g.output(readTimeout, top, "--no-optional-locks", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	names := strings.Split(string(out), "\x00")
	// Unmerged files list once per stage.
	slices.Sort(names)
	files := Changes{Top: top}
	for _, name := range slices.Compact(names) {
		if name == "" {
			continue
		}
		if info, err := g.lstat(filepath.Join(top, filepath.FromSlash(name))); err == nil && !info.IsDir() {
			files.Files = append(files.Files, FileChange{Path: name, OldPath: name, Status: 'A'})
		}
	}
	return files, nil
}

// Tracking is how a checkout's branch compares with its upstream.
type Tracking struct {
	// Branch is the checked out branch, empty for a detached HEAD, and
	// Upstream the branch it tracks, empty when it tracks none.
	Branch, Upstream string
	// Ahead counts the commits a push would send: those the upstream
	// lacks, or without one those no remote branch has. Behind counts
	// those a pull would bring.
	Ahead, Behind int
}

// Tracking compares the branch checked out at path with its upstream.
func (g Git) Tracking(path string) (Tracking, error) {
	var t Tracking
	t.Branch, _ = g.Run(path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if t.Branch == "" {
		return t, nil
	}
	if upstream, err := g.Run(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		t.Upstream = upstream
		out, err := g.Run(path, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		if err != nil {
			return t, err
		}
		if counts := strings.Fields(out); len(counts) == 2 {
			t.Ahead, _ = strconv.Atoi(counts[0])
			t.Behind, _ = strconv.Atoi(counts[1])
		}
		return t, nil
	}
	if g.Unborn(path) {
		return t, nil
	}
	out, err := g.Run(path, "rev-list", "--count", "HEAD", "--not", "--remotes")
	if err != nil {
		return t, err
	}
	t.Ahead, _ = strconv.Atoi(out)
	return t, nil
}

// GitDir returns the directory holding the refs and HEADs of the repository
// containing path, shared by all of its worktrees.
func (g Git) GitDir(path string) (string, error) {
	dir, err := g.Run(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Clean(dir), nil
}

// Fetch updates the remote branches and tags of every remote of the
// repository containing path, dropping remote branches their remote no
// longer has.
func (g Git) Fetch(path string) error {
	return g.network(path, "fetch", "--all", "--prune", "--quiet")
}

// FetchBranch updates one remote branch of the repository containing path,
// with the tags pointing into its commits.
func (g Git) FetchBranch(path, remote, branch string) error {
	return g.network(path, "fetch", "--quiet", remote, "refs/heads/"+branch)
}

// Remotes are the remotes and branches a fetch, pull or push chooses from.
type Remotes struct {
	// Current is the checked out branch, empty for a detached HEAD.
	Current string
	// Names lists the remotes, and Branches each one's branches by remote.
	Names    []string
	Branches map[string][]string
	// Local lists the local branches, and Upstreams the one each tracks on
	// a remote, by branch.
	Local     []string
	Upstreams map[string]RemoteBranch
}

// RemoteBranch is a branch of a remote.
type RemoteBranch struct {
	Remote, Branch string
}

// Remotes reads the remotes and branches of the repository containing path.
func (g Git) Remotes(path string) (Remotes, error) {
	r := Remotes{Branches: map[string][]string{}, Upstreams: map[string]RemoteBranch{}}
	r.Current, _ = g.Run(path, "symbolic-ref", "--quiet", "--short", "HEAD")
	out, err := g.Run(path, "remote")
	if err != nil {
		return r, err
	}
	r.Names = strings.Fields(out)
	out, err = g.Run(path, "for-each-ref", "--format=%(refname)%00%(upstream:remotename)%00%(upstream:remoteref)",
		"refs/heads", "refs/remotes")
	if err != nil {
		return r, err
	}
	for line := range strings.Lines(out) {
		fields := strings.Split(strings.TrimRight(line, "\n"), "\x00")
		if len(fields) < 3 {
			continue
		}
		if branch, ok := strings.CutPrefix(fields[0], "refs/heads/"); ok {
			r.Local = append(r.Local, branch)
			if upstream, ok := strings.CutPrefix(fields[2], "refs/heads/"); ok && slices.Contains(r.Names, fields[1]) {
				r.Upstreams[branch] = RemoteBranch{fields[1], upstream}
			}
			continue
		}
		name, _ := strings.CutPrefix(fields[0], "refs/remotes/")
		// Remote names may contain slashes, so the longest matching one
		// owns the branch.
		remote := ""
		for _, n := range r.Names {
			if strings.HasPrefix(name, n+"/") && len(n) > len(remote) {
				remote = n
			}
		}
		if branch := strings.TrimPrefix(name, remote+"/"); remote != "" && branch != "HEAD" {
			r.Branches[remote] = append(r.Branches[remote], branch)
		}
	}
	return r, nil
}

// DefaultRemote is the remote a branch without an upstream is published
// to: origin, else the only remote.
func (r Remotes) DefaultRemote() string {
	switch {
	case slices.Contains(r.Names, "origin"):
		return "origin"
	case len(r.Names) == 1:
		return r.Names[0]
	}
	return ""
}

// PullOptions choose what a pull brings and how.
type PullOptions struct {
	// Remote and Branch are where the commits come from.
	Remote, Branch string
	// Rebase replays the local commits onto the pulled ones instead of
	// merging, and Stash sets uncommitted changes aside meanwhile.
	Rebase, Stash bool
}

// Pull brings a remote branch's commits into the branch checked out at path,
// fast-forwarding when it can, else merging or rebasing as o chooses. A
// merge takes Git's default message without opening an editor.
func (g Git) Pull(path string, o PullOptions) error {
	args := []string{"pull", "--quiet", "--no-edit", "--no-rebase", "--no-autostash"}
	if o.Rebase {
		args[3] = "--rebase"
	}
	if o.Stash {
		args[4] = "--autostash"
	}
	return g.network(path, append(args, o.Remote, "refs/heads/"+o.Branch)...)
}

// PushOptions choose what a push sends and where.
type PushOptions struct {
	// Branch is the local branch, sent to To on Remote.
	Branch, Remote, To string
	// Tags also sends every tag, and Force replaces the remote branch even
	// when the push drops its commits, unless it changed since last fetched.
	Tags, Force bool
}

// PushBranch sends a local branch of the repository containing path to a
// remote branch. A branch without an upstream then tracks it.
func (g Git) PushBranch(path string, o PushOptions) error {
	if o.Branch == "" {
		return ErrDetached
	}
	if o.Remote == "" {
		return ErrNoRemote
	}
	args := []string{"push", "--quiet"}
	if o.Tags {
		args = append(args, "--tags")
	}
	if o.Force {
		args = append(args, "--force-with-lease")
	}
	if _, err := g.Run(path, "rev-parse", "--abbrev-ref", "--symbolic-full-name", o.Branch+"@{upstream}"); err != nil {
		args = append(args, "--set-upstream")
	}
	return g.network(path, append(args, o.Remote, "refs/heads/"+o.Branch+":refs/heads/"+o.To)...)
}

// DeleteBranch deletes a local branch of the repository containing path.
// Like git branch -d, it refuses a branch checked out in any worktree or
// whose commits neither its upstream nor the checked out branch contains.
func (g Git) DeleteBranch(path, branch string) error {
	_, err := g.Run(path, "branch", "--delete", branch)
	return err
}

// DeleteRemoteBranch deletes branch from remote and drops its remote
// branch. A branch the remote no longer has only loses its remote branch.
func (g Git) DeleteRemoteBranch(path, remote, branch string) error {
	err := g.network(path, "push", "--quiet", remote, "--delete", "refs/heads/"+branch)
	if err != nil && strings.Contains(err.Error(), "remote ref does not exist") {
		_, err = g.Run(path, "update-ref", "-d", "refs/remotes/"+remote+"/"+branch)
	}
	return err
}

// DeleteTag deletes a tag of the repository containing path.
func (g Git) DeleteTag(path, tag string) error {
	_, err := g.Run(path, "tag", "--delete", tag)
	return err
}

// DeleteRemoteTag deletes tag from remote, succeeding when the remote does
// not have it.
func (g Git) DeleteRemoteTag(path, remote, tag string) error {
	err := g.network(path, "push", "--quiet", remote, "--delete", "refs/tags/"+tag)
	if err != nil && strings.Contains(err.Error(), "remote ref does not exist") {
		return nil
	}
	return err
}
