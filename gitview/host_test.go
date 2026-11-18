package gitview

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

// countingHost runs git and reads files on this computer, counting what the
// view asked of it.
type countingHost struct {
	mu          sync.Mutex
	runs, reads int
}

func (h *countingHost) Run(ctx context.Context, dir string, env []string, stdin []byte, args ...string) ([]byte, []byte, error) {
	h.mu.Lock()
	h.runs++
	h.mu.Unlock()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), env...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (h *countingHost) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (h *countingHost) Readlink(path string) (string, error)   { return os.Readlink(path) }

func (h *countingHost) ReadFile(path string) ([]byte, error) {
	h.mu.Lock()
	h.reads++
	h.mu.Unlock()
	return os.ReadFile(path)
}

// githubHost is a countingHost that also runs the gh on env's PATH, with
// env, counting its runs.
type githubHost struct {
	countingHost
	env    []string
	ghRuns int
}

func (h *githubHost) GitHubCLI(context.Context) (string, error) {
	return Git{Env: h.env}.GitHubCLI(), nil
}

func (h *githubHost) RunGitHubCLI(ctx context.Context, dir string, env []string, args ...string) ([]byte, []byte, error) {
	h.mu.Lock()
	h.ghRuns++
	h.mu.Unlock()
	cmd := exec.CommandContext(ctx, Git{Env: h.env}.GitHubCLI(), args...)
	cmd.Dir, cmd.Env = dir, append(slices.Clone(h.env), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func TestHostRunsGitAndReadsFiles(t *testing.T) {
	repo := testRepo(t)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("a\nb\n"), 0644)
	host := &countingHost{}
	g := Git{Host: host}
	if top, err := g.Top(repo); err != nil || top == "" {
		t.Fatalf("Top = %q, %v", top, err)
	}
	changes, err := g.WorktreeChanges(repo, "")
	if err != nil || len(changes.Files) != 1 || changes.Files[0].Added != 2 {
		t.Fatalf("changes %+v, %v", changes, err)
	}
	_, after, err := g.FileVersions(changes, changes.Files[0])
	if err != nil || string(after) != "a\nb\n" {
		t.Fatalf("after %q, %v", after, err)
	}
	if host.runs == 0 || host.reads < 2 {
		t.Fatalf("host ran %d commands and read %d files", host.runs, host.reads)
	}
	if _, err := g.Run(repo, "rev-parse", "--verify", "missing"); err == nil {
		t.Fatal("a failed git reported no error")
	}

}
