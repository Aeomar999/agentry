package policy

import (
	"fmt"
	"strings"
)

// diffContext is how many unchanged lines a hunk shows around a change.
const diffContext = 3

// maxDiffCells bounds the line-matching table. Policy files are a few
// hundred lines; past this, the changed middle shows as all removed, then
// all added.
const maxDiffCells = 4_000_000

// LineDiff is a unified diff from old to new, line by line, or "" when they
// are the same.
func LineDiff(oldName, newName, old, new string) string {
	if old == new {
		return ""
	}
	ops := diffLines(splitLines(old), splitLines(new))
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)
	for _, h := range hunks(ops) {
		writeHunk(&out, ops[h.from:h.to])
	}
	return out.String()
}

// splitLines keeps each line's newline, so a last line without one is
// told apart from one with.
func splitLines(text string) []string {
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// diffOp is one line of the diff: kept (' '), removed ('-') or added ('+'),
// with its line number in the old and new text (counting from 1).
type diffOp struct {
	kind     byte
	line     string
	old, new int
}

// diffLines matches the lines the texts share, longest common subsequence
// first, after setting aside the lines both start and end with.
func diffLines(a, b []string) []diffOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	var ops []diffOp
	keep := func(i, j int) { ops = append(ops, diffOp{' ', a[i], i + 1, j + 1}) }
	for i := range prefix {
		keep(i, i)
	}
	ops = append(ops, matchMiddle(a, b, prefix, len(a)-suffix, len(b)-suffix)...)
	for k := suffix; k > 0; k-- {
		keep(len(a)-k, len(b)-k)
	}
	return ops
}

// matchMiddle diffs a[from:aEnd] against b[from:bEnd].
func matchMiddle(a, b []string, from, aEnd, bEnd int) []diffOp {
	n, m := aEnd-from, bEnd-from
	var ops []diffOp
	if n*m > maxDiffCells {
		for i := range n {
			ops = append(ops, diffOp{'-', a[from+i], from + i + 1, from + 1})
		}
		for j := range m {
			ops = append(ops, diffOp{'+', b[from+j], aEnd + 1, from + j + 1})
		}
		return ops
	}
	// common[i][j] is the length of the longest common subsequence of
	// a[from+i:aEnd] and b[from+j:bEnd].
	common := make([][]int, n+1)
	for i := range common {
		common[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[from+i] == b[from+j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[from+i] == b[from+j] && common[i][j] == common[i+1][j+1]+1:
			ops = append(ops, diffOp{' ', a[from+i], from + i + 1, from + j + 1})
			i, j = i+1, j+1
		case i < n && (j == m || common[i+1][j] >= common[i][j+1]):
			ops = append(ops, diffOp{'-', a[from+i], from + i + 1, from + j + 1})
			i++
		default:
			ops = append(ops, diffOp{'+', b[from+j], from + i + 1, from + j + 1})
			j++
		}
	}
	return ops
}

// span is a hunk: ops[from:to].
type span struct{ from, to int }

// hunks groups the changes with diffContext kept lines around each; changes
// closer than twice that share a hunk.
func hunks(ops []diffOp) []span {
	var all []span
	for i, op := range ops {
		if op.kind == ' ' {
			continue
		}
		from, to := max(0, i-diffContext), min(len(ops), i+1+diffContext)
		if last := len(all) - 1; last >= 0 && from <= all[last].to {
			all[last].to = to
		} else {
			all = append(all, span{from, to})
		}
	}
	return all
}

func writeHunk(out *strings.Builder, ops []diffOp) {
	var oldCount, newCount int
	for _, op := range ops {
		if op.kind != '+' {
			oldCount++
		}
		if op.kind != '-' {
			newCount++
		}
	}
	fmt.Fprintf(out, "@@ -%s +%s @@\n", hunkRange(ops[0].old, oldCount), hunkRange(ops[0].new, newCount))
	for _, op := range ops {
		out.WriteByte(op.kind)
		out.WriteString(op.line)
		if !strings.HasSuffix(op.line, "\n") {
			out.WriteString("\n\\ No newline at end of file\n")
		}
	}
}

// hunkRange is unified diff's start,count, where an empty range starts at
// the line before it.
func hunkRange(start, count int) string {
	if count == 0 {
		start--
	}
	if count == 1 {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}
