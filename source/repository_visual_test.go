package main

import "testing"

func graphCommit(hash string, parents ...string) commitEntry {
	return commitEntry{Hash: hash, Parents: parents}
}

func TestBuildGraphLinear(t *testing.T) {
	rows, columns := buildGraph([]commitEntry{
		graphCommit("c", "b"),
		graphCommit("b", "a"),
		graphCommit("a"),
	})
	if columns != 1 {
		t.Fatalf("columns: %d", columns)
	}
	for i, row := range rows {
		if row.CommitColumn != 0 || row.CommitLineID != 0 {
			t.Fatalf("row %d: %+v", i, row)
		}
		if len(row.Lines) != 1 {
			t.Fatalf("row %d lines: %+v", i, row.Lines)
		}
	}
	if rows[0].Lines[0].Top != -1 || rows[0].Lines[0].Bottom != 0 {
		t.Fatalf("tip line: %+v", rows[0].Lines[0])
	}
	if rows[2].Lines[0].Bottom != -1 {
		t.Fatalf("root line: %+v", rows[2].Lines[0])
	}
}

func TestBuildGraphMerge(t *testing.T) {
	rows, columns := buildGraph([]commitEntry{
		graphCommit("m", "a", "b"),
		graphCommit("a", "base"),
		graphCommit("b", "base"),
		graphCommit("base"),
	})
	if columns != 2 {
		t.Fatalf("columns: %d", columns)
	}
	// The merge opens a second line, to the right, for its second parent.
	if rows[0].CommitColumn != 0 || rows[0].CommitLineID != 0 {
		t.Fatalf("merge row: %+v", rows[0])
	}
	if len(rows[0].Lines) != 2 || rows[0].Lines[1].Bottom != 1 {
		t.Fatalf("merge lines: %+v", rows[0].Lines)
	}
	// Each parent keeps its own line and id until the two meet at base.
	if rows[1].CommitColumn != 0 || rows[1].CommitLineID != 0 {
		t.Fatalf("first parent row: %+v", rows[1])
	}
	if rows[2].CommitColumn != 1 || rows[2].CommitLineID != 1 {
		t.Fatalf("second parent row: %+v", rows[2])
	}
	if rows[3].CommitColumn != 0 || rows[3].CommitLineID != 0 {
		t.Fatalf("base row: %+v", rows[3])
	}
	if len(rows[3].Lines) != 2 {
		t.Fatalf("base lines: %+v", rows[3].Lines)
	}
	for _, line := range rows[3].Lines {
		if line.Column != 0 || line.Bottom != -1 {
			t.Fatalf("base line does not converge: %+v", line)
		}
	}
}

func TestBuildGraphDisjointTips(t *testing.T) {
	rows, columns := buildGraph([]commitEntry{
		graphCommit("x", "x1"),
		graphCommit("y", "y1"),
		graphCommit("x1"),
		graphCommit("y1"),
	})
	if columns != 2 {
		t.Fatalf("columns: %d", columns)
	}
	if rows[0].CommitColumn != 0 || rows[1].CommitColumn != 1 {
		t.Fatalf("tips: %+v %+v", rows[0], rows[1])
	}
	if rows[0].CommitLineID == rows[1].CommitLineID {
		t.Fatalf("tips share a line id: %+v", rows)
	}
}
