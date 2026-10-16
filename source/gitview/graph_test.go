package gitview

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildGraphJoinsBranchesAndMerges(t *testing.T) {
	commits := []Commit{
		{Hash: "m", Parents: []string{"a", "b"}},
		{Hash: "b", Parents: []string{"r"}},
		{Hash: "a", Parents: []string{"r"}},
		{Hash: "r"},
	}
	commits = append(commits, Commit{Hash: "x"})
	var got []string
	for _, row := range buildGraph(commits) {
		got = append(got, fmt.Sprintf("%d:%d/%d %v", row.Column, row.Line, row.Columns, row.Lines))
	}
	// The merge opens a line to its second parent, which ends at the root
	// beside the first parent's line. A later root needs only its own
	// column, however wide the rows above it are.
	want := []string{
		"0:0/2 [{0 -1 0 0} {1 -1 0 1}]",
		"1:1/2 [{0 0 0 0} {1 1 1 1}]",
		"0:0/2 [{0 0 0 0} {1 1 1 1}]",
		"0:0/2 [{0 0 0 -1} {1 1 0 -1}]",
		"0:2/1 []",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("graph:\n%s", strings.Join(got, "\n"))
	}
}

func TestSearchCommitsMatchesCoAuthors(t *testing.T) {
	commits := []Commit{
		{Hash: "a", Author: "Ann"},
		{Hash: "b", Author: "Bob", CoAuthors: []Person{{Name: "Claude", Email: "noreply@anthropic.com"}}},
	}
	if got := searchCommits(commits, "claude"); fmt.Sprint(got) != "[1]" {
		t.Fatalf("name: %v", got)
	}
	if got := searchCommits(commits, "Anthropic"); fmt.Sprint(got) != "[1]" {
		t.Fatalf("email: %v", got)
	}
}

func TestRefRemoteMarksRemoteBranches(t *testing.T) {
	remotes := map[string]bool{"origin": true, "mirror": false, "mirror/eu": true}
	for ref, want := range map[string]string{"origin/main": "origin", "mirror/eu/main": "mirror/eu", "mirror/main": "mirror", "main": "", "originx/main": ""} {
		if got := refRemote(remotes, ref); got != want {
			t.Fatalf("%s: %q, want %q", ref, got, want)
		}
	}
	if remoteIcon(remotes, "origin")() != githubIcon() || remoteIcon(remotes, "mirror")() == githubIcon() {
		t.Fatal("remote icons")
	}
}

func TestFoldCommitsHidesMergedCommits(t *testing.T) {
	commits := []Commit{
		{Hash: "m2", Parents: []string{"m1", "t"}},
		{Hash: "t", Parents: []string{"s1"}},
		{Hash: "m1", Parents: []string{"a", "s2"}},
		{Hash: "s2", Parents: []string{"s1"}},
		{Hash: "s1", Parents: []string{"r"}},
		{Hash: "a", Parents: []string{"r"}},
		{Hash: "r"},
	}
	// m1 folds s2, but not s1, which t builds on outside the merge.
	if got := foldCommits(commits, map[string]bool{"m1": true}); fmt.Sprint(got) != "map[s2:m1]" {
		t.Fatalf("m1: %v", got)
	}
	// m2 folds only t: its first parent reaches s1 through m1.
	if got := foldCommits(commits, map[string]bool{"m2": true}); fmt.Sprint(got) != "map[t:m2]" {
		t.Fatalf("m2: %v", got)
	}
}
