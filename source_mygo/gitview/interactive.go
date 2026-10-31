package gitview

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// RebaseAction is what an interactive rebase does with a commit.
type RebaseAction int

const (
	RebasePick RebaseAction = iota
	RebaseReword
	RebaseEdit
	RebaseSquash
	RebaseFixup
	RebaseDrop
)

// RebaseCommit is one commit of an interactive rebase and what to do with
// it. Message replaces a reworded commit's message.
type RebaseCommit struct {
	Commit  Commit
	Action  RebaseAction
	Message string
}

// RebaseCommits lists the commits from base to HEAD, oldest first, that an
// interactive rebase would edit.
func (g Git) RebaseCommits(path, base string) ([]Commit, error) {
	out, err := g.output(historyTimeout, path, "--no-optional-locks", "log", "--reverse", "-z",
		"--pretty=format:"+commitFormat, base+"..HEAD")
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

// InteractiveRebase replays commits oldest first as their actions say,
// starting from base. It needs a local git executable: the todo and message
// editors are temporary scripts this computer runs.
func (g Git) InteractiveRebase(path, base string, commits []RebaseCommit, autostash bool) error {
	if g.Host != nil {
		return errors.New("interactive rebase needs the local Git installation")
	}
	if len(commits) == 0 {
		return errors.New("no commits to rebase")
	}
	var todo strings.Builder
	var rewords []RebaseCommit
	for i, rc := range commits {
		action := "pick"
		switch rc.Action {
		case RebaseReword:
			action = "reword"
			rewords = append(rewords, rc)
		case RebaseEdit:
			action = "edit"
		case RebaseSquash:
			if i == 0 {
				return errors.New("the first commit cannot be squashed")
			}
			action = "squash"
		case RebaseFixup:
			if i == 0 {
				return errors.New("the first commit cannot be fixed up")
			}
			action = "fixup"
		case RebaseDrop:
			action = "drop"
		}
		todo.WriteString(action + " " + rc.Commit.Hash + " " + strings.ReplaceAll(rc.Commit.Subject, "\n", " ") + "\n")
	}
	dir, err := os.MkdirTemp("", "onegit-rebase-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	todoPath := filepath.Join(dir, "todo")
	if err := os.WriteFile(todoPath, []byte(todo.String()), 0o600); err != nil {
		return err
	}
	sequence := filepath.Join(dir, "sequence.sh")
	sequenceBody := "#!/bin/sh\ncat " + shellQuote(todoPath) + " > \"$1\"\n"
	if err := os.WriteFile(sequence, []byte(sequenceBody), 0o700); err != nil {
		return err
	}
	editor := filepath.Join(dir, "editor.sh")
	var script strings.Builder
	script.WriteString("#!/bin/sh\nsubject=$(head -n 1 \"$1\")\ncase \"$subject\" in\n")
	for _, rc := range rewords {
		message := rc.Message
		if strings.TrimSpace(message) == "" {
			message = fullMessage(rc.Commit)
		}
		script.WriteString(shellQuote(rc.Commit.Subject) + ")\n")
		script.WriteString("printf '%s\n' " + shellQuote(strings.TrimRight(message, "\n")) + " > \"$1\"\n;;\n")
	}
	script.WriteString("esac\n")
	if err := os.WriteFile(editor, []byte(script.String()), 0o700); err != nil {
		return err
	}
	env := []string{
		"GIT_SEQUENCE_EDITOR=sh " + shellQuote(filepath.ToSlash(sequence)),
		"GIT_EDITOR=sh " + shellQuote(filepath.ToSlash(editor)),
	}
	args := []string{"rebase", "--interactive", "--no-autostash"}
	if autostash {
		args[2] = "--autostash"
	}
	args = append(args, base)
	_, err = g.withEnv(env...).runFor(networkTimeout, path, args...)
	return err
}

// fullMessage is a commit's subject and body, with the body when it has one.
func fullMessage(c Commit) string {
	if c.Body == "" {
		return c.Subject
	}
	return c.Subject + "\n\n" + c.Body
}

// shellQuote quotes s for a POSIX shell, including as a case pattern.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
