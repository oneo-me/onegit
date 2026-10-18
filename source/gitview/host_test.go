package gitview

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// countingHost runs git and reads files on this computer, counting what the
// view asked of it.
type countingHost struct {
	mu            sync.Mutex
	runs, reads   int
	watched       []string
	watchFailures int
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

func (h *countingHost) Watch(_ context.Context, dir string, repository bool) (Watcher, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watched = append(h.watched, dir)
	h.watchFailures++
	return nil, errors.New("no watch")
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

	// The view's watches go to the host too.
	v, pump := testView(t, Options{Git: g, Watch: true})
	v.SetPath(repo)
	pump(func() bool { return v.repo.loaded && !v.repo.loading })
	pump(func() bool {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.watchFailures > 0
	})
}

func TestWatchIgnored(t *testing.T) {
	for path, want := range map[string]bool{"objects": true, "refs": false, "logs": true} {
		if WatchIgnored(true, path) != want {
			t.Errorf("repository %q ignored = %v", path, !want)
		}
	}
	for path, want := range map[string]bool{"a/node_modules": true, "sub/.git": true, "src/main.go": false} {
		if WatchIgnored(false, path) != want {
			t.Errorf("worktree %q ignored = %v", path, !want)
		}
	}
}
