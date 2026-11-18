package gitview

import (
	"context"
	"io/fs"
	"os"
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
}

// GitHubHost is a Host that also runs the GitHub CLI, gh, there, so a push
// from a repository without remotes creates a GitHub repository as it does
// on this computer. A Host without it keeps the plain notice.
type GitHubHost interface {
	Host
	// GitHubCLI returns the path of gh on the host, empty when it has none.
	GitHubCLI(ctx context.Context) (string, error)
	// RunGitHubCLI runs gh in dir with args and env added to its
	// environment, returning what Run returns for git.
	RunGitHubCLI(ctx context.Context, dir string, env []string, args ...string) (stdout, stderr []byte, err error)
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
