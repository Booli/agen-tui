package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/pimrutgers/agen-tui/internal/theme"
)

// DiffMaxLines bounds the LCS diff. Longer inputs fall back to a
// "delete-block then add-block" rendering so we don't pay O(n*m) on
// huge edits.
const DiffMaxLines = 200

// DiffRow is a single output row in a line-level diff.
type DiffRow struct {
	Kind byte // ' ', '-', '+'
	Text string
}

// RenderDiff returns coloured `-`/`+`/context lines for old→neu at
// line granularity.
func RenderDiff(old, neu string, width int) []string {
	a := strings.Split(old, "\n")
	b := strings.Split(neu, "\n")
	if len(a) > DiffMaxLines || len(b) > DiffMaxLines {
		return renderDiffBlocks(a, b, width)
	}
	rows := LCSDiff(a, b)
	var out []string
	for _, r := range rows {
		out = append(out, RenderDiffRow(r.Kind, r.Text, width))
	}
	return out
}

// LCSDiff computes a line-level diff between a and b using LCS
// backtracking. It returns rows in source order: ' ' (context), '-'
// (only in a), '+' (only in b).
func LCSDiff(a, b []string) []DiffRow {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	var rev []DiffRow
	for i, j := n, m; i > 0 || j > 0; {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			rev = append(rev, DiffRow{' ', a[i-1]})
			i--
			j--
		case j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]):
			rev = append(rev, DiffRow{'+', b[j-1]})
			j--
		default:
			rev = append(rev, DiffRow{'-', a[i-1]})
			i--
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func renderDiffBlocks(a, b []string, width int) []string {
	var out []string
	for _, ln := range a {
		out = append(out, RenderDiffRow('-', ln, width))
	}
	for _, ln := range b {
		out = append(out, RenderDiffRow('+', ln, width))
	}
	return out
}

// RenderDiffRow formats a single diff row with the appropriate colour.
func RenderDiffRow(kind byte, text string, width int) string {
	var style lipgloss.Style
	switch kind {
	case '-':
		style = theme.DiffDel
	case '+':
		style = theme.DiffAdd
	default:
		style = theme.Muted
	}
	body := TruncRunes(text, width-3)
	return " " + style.Render(string(kind)+" ") + style.Render(body)
}

// RenderUnifiedDiff colours `git diff` output: + green, - red, @@
// muted. Drops noisy headers (`diff --git`, `index`, `---`, `+++`,
// mode lines, similarity/rename headers).
func RenderUnifiedDiff(diff string, width int) []string {
	var out []string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git"),
			strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "),
			strings.HasPrefix(line, "new file mode"),
			strings.HasPrefix(line, "deleted file mode"),
			strings.HasPrefix(line, "similarity index"),
			strings.HasPrefix(line, "rename "):
			continue
		case strings.HasPrefix(line, "@@"):
			out = append(out, " "+theme.Muted.Render(TruncRunes(line, width-1)))
		case strings.HasPrefix(line, "+"):
			out = append(out, " "+theme.DiffAdd.Render(TruncRunes(line, width-1)))
		case strings.HasPrefix(line, "-"):
			out = append(out, " "+theme.DiffDel.Render(TruncRunes(line, width-1)))
		default:
			out = append(out, " "+TruncRunes(line, width-1))
		}
	}
	return out
}
