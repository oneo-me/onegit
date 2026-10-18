package gitview

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// Host is where repositories live when they are not on this computer, such
// as another computer a host app reaches over SSH. It runs git there and
// reads its files; paths are that computer's.
type Host interface {
	// Run runs git in dir with args, stdin as its standard input unless
	// nil and env added to its environment. It returns git's standard
	// output and error output, and an error when git could not run or
	// exited with one.
	Run(ctx context.Context, dir string, env []string, stdin []byte, args ...string) (stdout, stderr []byte, err error)
	// Lstat, ReadFile and Readlink read the host's files as the os
	// functions of the same names do.
	Lstat(path string) (fs.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	Readlink(path string) (string, error)
	// Watch follows changes under dir until ctx ends: a repository's
	// metadata directory with repository set, which leaves out what
	// WatchIgnored does, else a worktree.
	Watch(ctx context.Context, dir string, repository bool) (Watcher, error)
}

// Watcher reports changes under a watched directory. Next waits for the
// next batch of them; it fails once the watch ended.
type Watcher interface {
	Next(ctx context.Context) error
	Close() error
}

// WatchIgnored reports a path a watch leaves out, by its path relative to
// the watched directory, separated by slashes: in a repository's metadata,
// its object database and other data that matters only once a ref points
// at it; in a worktree, nested repositories' metadata and node_modules.
func WatchIgnored(repository bool, path string) bool {
	if repository {
		switch path {
		case "objects", "lfs", "logs", "hooks", "info":
			return true
		}
		return false
	}
	name := path[strings.LastIndex(path, "/")+1:]
	return name == ".git" || name == "node_modules"
}

func (g Git) lstat(path string) (fs.FileInfo, error) {
	if g.Host != nil {
		return g.Host.Lstat(path)
	}
	return os.Lstat(path)
}

// stat is os.Stat on this computer; a host's links are not followed.
func (g Git) stat(path string) (fs.FileInfo, error) {
	if g.Host != nil {
		return g.Host.Lstat(path)
	}
	return os.Stat(path)
}

func (g Git) readFile(path string) ([]byte, error) {
	if g.Host != nil {
		return g.Host.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (g Git) readlink(path string) (string, error) {
	if g.Host != nil {
		return g.Host.Readlink(path)
	}
	return os.Readlink(path)
}

// watch follows dir on this computer with MyGo's WatchFiles, or on the
// host.
func (g Git) watch(ctx context.Context, dir string, repository bool) (Watcher, error) {
	if g.Host != nil {
		return g.Host.Watch(ctx, dir, repository)
	}
	w, err := mygo.WatchFiles(ctx, dir, mygo.FileWatchOptions{
		Recursive: true, HintsOnly: true,
		Ignore:   func(path string, _ bool) bool { return WatchIgnored(repository, path) },
		Debounce: 300 * time.Millisecond, MaxDelay: 2 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return localWatcher{w}, nil
}

type localWatcher struct{ w *mygo.FileWatcher }

func (l localWatcher) Next(ctx context.Context) error {
	_, err := l.w.Next(ctx)
	return err
}

func (l localWatcher) Close() error { return l.w.Close() }
