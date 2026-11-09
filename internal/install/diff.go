package install

import (
	"fmt"
	"strings"
)

const (
	diffContext  = 2
	maxDiffLines = 2000
	maxDiffShown = 200
)

// lineDiff renders changed lines with a little context ("-" old, "+" new).
// It is a review aid for small config files, not a general diff tool.
func lineDiff(old, new string) string {
	if old == new {
		return ""
	}
	a, b := splitLines(old), splitLines(new)
	if len(a) > maxDiffLines || len(b) > maxDiffLines {
		return fmt.Sprintf("(%d lines -> %d lines; too large to show)", len(a), len(b))
	}
	ops := diffOps(a, b)
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op[0] != ' ' {
			for k := max(0, i-diffContext); k <= min(len(ops)-1, i+diffContext); k++ {
				keep[k] = true
			}
		}
	}
	var out []string
	for i, op := range ops {
		if keep[i] {
			out = append(out, op)
		} else if len(out) > 0 && out[len(out)-1] != "..." {
			out = append(out, "...")
		}
	}
	if len(out) > maxDiffShown {
		out = append(out[:maxDiffShown], "... (truncated)")
	}
	return strings.Join(out, "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// diffOps walks a longest-common-subsequence table.
func diffOps(a, b []string) []string {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []string
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops, i, j = append(ops, " "+a[i]), i+1, j+1
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			ops, j = append(ops, "+"+b[j]), j+1
		default:
			ops, i = append(ops, "-"+a[i]), i+1
		}
	}
	return ops
}
