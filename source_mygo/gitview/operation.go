package gitview

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// OperationKind is a Git operation that stopped, leaving the repository
// mid-way until the user completes or aborts it.
type OperationKind int

const (
	OperationNone OperationKind = iota
	OperationMerge
	OperationRebase
	OperationAm
	OperationCherryPick
	OperationRevert
	OperationSquash
	OperationBisect
)

// Operation is the state a repository is in between a Git write and its
// completion, such as a merge with conflicts.
type Operation struct {
	Kind OperationKind
}

// Operation reads whether the repository containing path stopped in a
// merge, rebase, am, cherry-pick, revert or squash.
func (g Git) Operation(path string) (Operation, error) {
	dir, err := g.Run(path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return Operation{}, err
	}
	exists := func(name string) bool {
		_, err := g.stat(filepath.Join(dir, name))
		return err == nil
	}
	dirExists := func(name, marker string) bool {
		p := filepath.Join(dir, name)
		if _, err := g.stat(p); err != nil {
			return false
		}
		if marker == "" {
			return true
		}
		_, err := g.stat(filepath.Join(p, marker))
		return err == nil
	}
	switch {
	case dirExists("rebase-merge", ""):
		return Operation{Kind: OperationRebase}, nil
	case dirExists("rebase-apply", "applying"):
		return Operation{Kind: OperationAm}, nil
	case dirExists("rebase-apply", ""):
		return Operation{Kind: OperationRebase}, nil
	case exists("MERGE_HEAD"):
		return Operation{Kind: OperationMerge}, nil
	case exists("CHERRY_PICK_HEAD"):
		return Operation{Kind: OperationCherryPick}, nil
	case exists("REVERT_HEAD"):
		return Operation{Kind: OperationRevert}, nil
	case dirExists("sequencer", ""):
		return Operation{Kind: OperationCherryPick}, nil
	case exists("SQUASH_MSG"):
		return Operation{Kind: OperationSquash}, nil
	case exists("BISECT_LOG"):
		return Operation{Kind: OperationBisect}, nil
	}
	return Operation{}, nil
}

// ContinueOperation finishes the operation that stopped in the worktree at
// path, using the message Git staged for it. It is an error while conflicts
// remain.
func (g Git) ContinueOperation(path string) error {
	op, err := g.Operation(path)
	if err != nil {
		return err
	}
	editor := g.withEnv("GIT_EDITOR=true")
	switch op.Kind {
	case OperationMerge, OperationSquash:
		_, err = editor.runFor(networkTimeout, path, "commit", "--quiet", "--no-edit")
	case OperationRebase:
		_, err = editor.runFor(networkTimeout, path, "rebase", "--continue")
	case OperationAm:
		_, err = editor.runFor(networkTimeout, path, "am", "--continue")
	case OperationCherryPick:
		_, err = editor.runFor(networkTimeout, path, "cherry-pick", "--continue")
	case OperationRevert:
		_, err = editor.runFor(networkTimeout, path, "revert", "--continue")
	case OperationBisect:
		_, err = g.runFor(networkTimeout, path, "bisect", "reset")
	default:
		return errors.New("no Git operation to continue")
	}
	return err
}

// AbortOperation undoes the operation that stopped in the worktree at path,
// which loses the changes it made.
func (g Git) AbortOperation(path string) error {
	op, err := g.Operation(path)
	if err != nil {
		return err
	}
	switch op.Kind {
	case OperationMerge:
		_, err = g.runFor(networkTimeout, path, "merge", "--abort")
	case OperationRebase:
		_, err = g.runFor(networkTimeout, path, "rebase", "--abort")
	case OperationAm:
		_, err = g.runFor(networkTimeout, path, "am", "--abort")
	case OperationCherryPick:
		_, err = g.runFor(networkTimeout, path, "cherry-pick", "--abort")
	case OperationRevert:
		_, err = g.runFor(networkTimeout, path, "revert", "--abort")
	case OperationSquash:
		_, err = g.runFor(networkTimeout, path, "reset", "--merge", "ORIG_HEAD")
	case OperationBisect:
		_, err = g.runFor(networkTimeout, path, "bisect", "reset")
	default:
		return errors.New("no Git operation to abort")
	}
	return err
}

// ConflictSide is the version a conflict is resolved with.
type ConflictSide int

const (
	ConflictOurs ConflictSide = iota
	ConflictTheirs
)

// ResolveConflict takes one side of a conflicted file and stages it,
// marking the conflict resolved. The file's same path is kept; a conflict
// where that side deleted the file is left for Git to report.
func (g Git) ResolveConflict(top, path string, side ConflictSide) error {
	flag := "--ours"
	if side == ConflictTheirs {
		flag = "--theirs"
	}
	if _, err := g.Run(top, "checkout", flag, "--", path); err != nil {
		return err
	}
	_, err := g.Run(top, "--literal-pathspecs", "add", "--", path)
	return err
}

// WorktreeFile reads path, relative to the worktree at top, from its
// worktree file.
func (g Git) WorktreeFile(top, path string) ([]byte, error) {
	return g.readFile(filepath.Join(top, filepath.FromSlash(path)))
}

// FileWriter is a Host that can write a worktree file, as discarding part
// of a change needs. A Host without it can still read and run Git.
type FileWriter interface {
	WriteFile(path string, data []byte, perm fs.FileMode) error
}

// DiscardText writes data as path, relative to the worktree at top, keeping
// the mode Git records for the file.
func (g Git) DiscardText(top, path string, data []byte) error {
	full := filepath.Join(top, filepath.FromSlash(path))
	if info, err := g.lstat(full); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("cannot discard part of a symbolic link")
	}
	mode, err := g.fileMode(top, path)
	if err != nil {
		return err
	}
	perm := fs.FileMode(0o644)
	if mode == "100755" {
		perm = 0o755
	}
	return g.writeFile(full, data, perm)
}

func (g Git) writeFile(path string, data []byte, perm fs.FileMode) error {
	if g.Host != nil {
		if writer, ok := g.Host.(FileWriter); ok {
			return writer.WriteFile(path, data, perm)
		}
		return errors.New("the host cannot write worktree files")
	}
	return os.WriteFile(path, data, perm)
}

// AmendCommit replaces the checked out commit with one holding the staged
// changes and message.
func (g Git) AmendCommit(top, message string) error {
	_, err := g.Run(top, "commit", "--quiet", "--amend", "-m", message)
	return err
}
