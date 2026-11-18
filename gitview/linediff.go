package gitview

import "bytes"

// lineStats counts the lines a shortest edit from before to after adds and
// deletes, splitting both on \n as Git's line counts do.
func lineStats(before, after []byte) (added, deleted int) {
	a, b := splitLines(before), splitLines(after)
	// Lines both ends share take no part in the edit.
	for len(a) > 0 && len(b) > 0 && bytes.Equal(a[0], b[0]) {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && bytes.Equal(a[len(a)-1], b[len(b)-1]) {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	d := editDistance(a, b)
	// d = deleted + added and len(b)-len(a) = added - deleted.
	added = (d + len(b) - len(a)) / 2
	return added, d - added
}

func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	return bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
}

// editDistance is the fewest lines deleted from a and added to b that turn
// one into the other, by Myers' algorithm.
func editDistance(a, b [][]byte) int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return n + m
	}
	offset := n + m
	v := make([]int, 2*offset+2)
	for d := 0; d <= n+m; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && v[offset+k-1] < v[offset+k+1] {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && bytes.Equal(a[x], b[y]) {
				x, y = x+1, y+1
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return d
			}
		}
	}
	return n + m
}

// lineHunk is a run of changed lines: lines OldStart to OldEnd of the old
// text, from 0 and excluded at the end, give way to NewStart to NewEnd of
// the new one.
type lineHunk struct{ OldStart, OldEnd, NewStart, NewEnd int }

// maxEditLines bounds the edit lineHunks traces; a larger edit is one hunk.
const maxEditLines = 20000

// lineHunks lists the runs of lines a shortest edit from a to b changes, by
// Myers' algorithm.
func lineHunks(a, b [][]byte) []lineHunk {
	n, m := len(a), len(b)
	offset := n + m + 1
	v := make([]int, 2*offset+1)
	var trace [][]int
	d := 0
search:
	for ; d <= n+m; d++ {
		if d > maxEditLines {
			return []lineHunk{{0, n, 0, m}}
		}
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && v[offset+k-1] < v[offset+k+1] {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && bytes.Equal(a[x], b[y]) {
				x, y = x+1, y+1
			}
			v[offset+k] = x
			if x >= n && y >= m {
				break search
			}
		}
	}
	// Walk the trace back, marking the lines each step deletes or adds.
	deleted, added := make([]bool, n), make([]bool, m)
	x, y := n, m
	for ; d > 0; d-- {
		prev := trace[d]
		k := x - y
		var pk int
		if k == -d || k != d && prev[offset+k-1] < prev[offset+k+1] {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := prev[offset+pk]
		py := px - pk
		for x > px && y > py {
			x, y = x-1, y-1
		}
		if x == px {
			added[py] = true
		} else {
			deleted[px] = true
		}
		x, y = px, py
	}
	var hunks []lineHunk
	i, j := 0, 0
	for i < n || j < m {
		if i < n && j < m && !deleted[i] && !added[j] {
			i, j = i+1, j+1
			continue
		}
		h := lineHunk{OldStart: i, NewStart: j}
		for i < n && deleted[i] {
			i++
		}
		for j < m && added[j] {
			j++
		}
		h.OldEnd, h.NewEnd = i, j
		hunks = append(hunks, h)
	}
	return hunks
}
